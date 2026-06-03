package app

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"

	locationdomain "github.com/tinoosan/agen8-mcp-server/internal/services/location/domain"
)

type Service struct {
	locations locationdomain.Repository
	transport Transport
	projects  ProjectReferenceChecker
	clock     Clock
	hostname  HostnameResolver
	logger    *slog.Logger
	bridgeMu  sync.Mutex
	bridges   map[locationdomain.ID]*sync.Mutex
}

type Config struct {
	Locations locationdomain.Repository
	Transport Transport
	Projects  ProjectReferenceChecker
	Clock     Clock
	Hostname  HostnameResolver
	Logger    *slog.Logger
}

func NewService(cfg Config) (*Service, error) {
	if cfg.Locations == nil {
		return nil, fmt.Errorf("location repository is required")
	}
	if cfg.Transport == nil {
		return nil, fmt.Errorf("location transport is required")
	}
	if cfg.Projects == nil {
		return nil, fmt.Errorf("location project reference checker is required")
	}
	clock := cfg.Clock
	if clock == nil {
		clock = systemClock{}
	}
	hostname := cfg.Hostname
	if hostname == nil {
		hostname = os.Hostname
	}
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default().With("service", "location")
	}
	return &Service{
		locations: cfg.Locations,
		transport: cfg.Transport,
		projects:  cfg.Projects,
		clock:     clock,
		hostname:  hostname,
		logger:    logger,
		bridges:   map[locationdomain.ID]*sync.Mutex{},
	}, nil
}

type systemClock struct{}

func (systemClock) Now() time.Time {
	return time.Now().UTC()
}

type SaveLocationInput struct {
	ID      locationdomain.ID
	Kind    locationdomain.Kind
	Label   string
	Address locationdomain.Address
	Status  locationdomain.Status
	Ready   bool
}

type CreateLocationInput struct {
	Kind          locationdomain.Kind
	Label         string
	Address       locationdomain.Address
	CredentialRef string
}

type UpdateLocationInput struct {
	ID            locationdomain.ID
	Label         string
	Address       *locationdomain.Address
	CredentialRef *string
}

func (s *Service) EnsureLocal(ctx context.Context) (locationdomain.Location, error) {
	if s == nil {
		return locationdomain.Location{}, fmt.Errorf("location service is nil")
	}
	current, err := s.GetLocation(ctx, "local")
	if err == nil {
		return s.ensureLocalHost(ctx, current)
	}
	if !errors.Is(err, locationdomain.ErrNotFound) {
		return locationdomain.Location{}, err
	}
	now := s.now()
	record := locationdomain.Record{
		ID:        "local",
		Kind:      locationdomain.KindLocal,
		Label:     "This machine",
		Address:   s.localAddress(),
		Status:    locationdomain.StatusNotReady,
		CreatedAt: now,
		UpdatedAt: now,
	}
	saved, err := s.locations.Save(ctx, record)
	if err != nil {
		return locationdomain.Location{}, err
	}
	location, err := locationdomain.Wrap(saved)
	if err != nil {
		return locationdomain.Location{}, err
	}
	probed, err := s.ProbeLocation(ctx, location.ID())
	if err == nil {
		return probed, nil
	}
	current, getErr := s.GetLocation(ctx, location.ID())
	if getErr != nil {
		return locationdomain.Location{}, err
	}
	return current, nil
}

func (s *Service) CreateLocation(ctx context.Context, input CreateLocationInput) (locationdomain.Location, error) {
	if s == nil {
		return locationdomain.Location{}, fmt.Errorf("location service is nil")
	}
	kind := locationdomain.Kind(strings.TrimSpace(string(input.Kind)))
	if kind == "" {
		return locationdomain.Location{}, fmt.Errorf("location kind is required")
	}
	id := locationIDForInput(kind, input.Label, input.Address)
	now := s.now()
	record := locationdomain.Record{
		ID:            id,
		Kind:          kind,
		Label:         strings.TrimSpace(input.Label),
		Address:       input.Address,
		CredentialRef: strings.TrimSpace(input.CredentialRef),
		Status:        locationdomain.StatusNotReady,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if kind == locationdomain.KindLocal {
		record.ID = "local"
		record.Address = s.localAddress()
		if record.Label == "" {
			record.Label = "This machine"
		}
	}
	saved, err := s.locations.Save(ctx, record)
	if err != nil {
		return locationdomain.Location{}, err
	}
	location, err := locationdomain.Wrap(saved)
	if err != nil {
		return locationdomain.Location{}, err
	}
	return s.ProbeLocation(ctx, location.ID())
}

func (s *Service) SaveLocation(ctx context.Context, input SaveLocationInput) (locationdomain.Location, error) {
	if s == nil {
		return locationdomain.Location{}, fmt.Errorf("location service is nil")
	}
	now := s.now()
	record := locationdomain.Record{
		ID:        input.ID,
		Kind:      input.Kind,
		Label:     strings.TrimSpace(input.Label),
		Address:   input.Address,
		Status:    input.Status,
		Ready:     input.Ready,
		CreatedAt: now,
		UpdatedAt: now,
	}
	saved, err := s.locations.Save(ctx, record)
	if err != nil {
		return locationdomain.Location{}, err
	}
	return locationdomain.Wrap(saved)
}

func (s *Service) UpdateLocation(ctx context.Context, input UpdateLocationInput) (locationdomain.Location, error) {
	if s == nil {
		return locationdomain.Location{}, fmt.Errorf("location service is nil")
	}
	current, err := s.GetLocation(ctx, input.ID)
	if err != nil {
		return locationdomain.Location{}, err
	}
	record := current.Record()
	if strings.TrimSpace(input.Label) != "" {
		record.Label = strings.TrimSpace(input.Label)
	}
	if input.Address != nil {
		record.Address = *input.Address
	}
	if input.CredentialRef != nil {
		record.CredentialRef = strings.TrimSpace(*input.CredentialRef)
	}
	record.Status = locationdomain.StatusNotReady
	record.Ready = false
	record.UpdatedAt = s.now()
	saved, err := s.locations.Save(ctx, record)
	if err != nil {
		return locationdomain.Location{}, err
	}
	return locationdomain.Wrap(saved)
}

func (s *Service) GetLocation(ctx context.Context, id locationdomain.ID) (locationdomain.Location, error) {
	if s == nil {
		return locationdomain.Location{}, fmt.Errorf("location service is nil")
	}
	id = locationdomain.ID(strings.TrimSpace(string(id)))
	if id == "" {
		return locationdomain.Location{}, fmt.Errorf("location id is required")
	}
	record, err := s.locations.Get(ctx, id)
	if err != nil {
		return locationdomain.Location{}, err
	}
	return locationdomain.Wrap(record)
}

func (s *Service) ListLocations(ctx context.Context, filter locationdomain.Filter) ([]locationdomain.Location, error) {
	if s == nil {
		return nil, fmt.Errorf("location service is nil")
	}
	if filter.Limit < 0 || filter.Offset < 0 {
		return nil, fmt.Errorf("location limit and offset must be non-negative")
	}
	records, err := s.locations.List(ctx, filter)
	if err != nil {
		return nil, err
	}
	out := make([]locationdomain.Location, 0, len(records))
	for _, record := range records {
		location, err := locationdomain.Wrap(record)
		if err != nil {
			return nil, err
		}
		out = append(out, location)
	}
	return out, nil
}

func (s *Service) DeleteLocation(ctx context.Context, id locationdomain.ID) error {
	if s == nil {
		return fmt.Errorf("location service is nil")
	}
	id = locationdomain.ID(strings.TrimSpace(string(id)))
	if id == "" {
		return fmt.Errorf("location id is required")
	}
	hasProjects, err := s.projects.HasProjectsForLocation(ctx, id)
	if err != nil {
		return err
	}
	if hasProjects {
		return fmt.Errorf("location %q has active projects", id)
	}
	return s.locations.Delete(ctx, id)
}

func (s *Service) ProbeLocation(ctx context.Context, id locationdomain.ID) (locationdomain.Location, error) {
	if s == nil {
		return locationdomain.Location{}, fmt.Errorf("location service is nil")
	}
	location, err := s.GetLocation(ctx, id)
	if err != nil {
		return locationdomain.Location{}, err
	}
	result, err := s.transport.Probe(ctx, location)
	record := location.Record()
	record.UpdatedAt = s.now()
	if err != nil {
		record.Status = locationdomain.StatusOffline
		record.Ready = false
		record.Probe = locationdomain.Probe{}
		record.LastProbeError = err.Error()
		probedAt := s.now()
		record.LastProbedAt = &probedAt
	} else {
		record.Probe = locationdomain.Probe{
			Reachable:    result.Reachable,
			FileBrowsing: result.FileBrowsing,
			Exec:         result.Exec,
			Codex:        result.Codex,
			Claude:       result.Claude,
		}
		record.Ready = result.Reachable && result.FileBrowsing && result.Exec && result.Codex
		if record.Ready {
			record.Status = locationdomain.StatusOnline
		} else {
			record.Status = locationdomain.StatusNotReady
		}
		record.LastProbeError = strings.TrimSpace(result.Message)
		probedAt := result.ProbedAt
		if probedAt.IsZero() {
			probedAt = s.now()
		}
		probedAt = probedAt.UTC()
		record.LastProbedAt = &probedAt
	}
	saved, saveErr := s.locations.Save(ctx, record)
	if saveErr != nil {
		return locationdomain.Location{}, saveErr
	}
	if err != nil {
		return locationdomain.Location{}, err
	}
	return locationdomain.Wrap(saved)
}

func (s *Service) ListDir(ctx context.Context, locationID locationdomain.ID, path string) ([]DirEntry, error) {
	if s == nil {
		return nil, fmt.Errorf("location service is nil")
	}
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("path is required")
	}
	location, err := s.GetLocation(ctx, locationID)
	if err != nil {
		return nil, err
	}
	if !location.Ready() {
		return nil, fmt.Errorf("location %q is not ready", location.ID())
	}
	return s.transport.ListDir(ctx, location, strings.TrimSpace(path))
}

func (s *Service) InstallCodex(ctx context.Context, locationID locationdomain.ID) (locationdomain.Location, error) {
	if s == nil {
		return locationdomain.Location{}, fmt.Errorf("location service is nil")
	}
	location, err := s.GetLocation(ctx, locationID)
	if err != nil {
		return locationdomain.Location{}, err
	}
	if _, err := s.transport.InstallCodex(ctx, location); err != nil {
		return locationdomain.Location{}, err
	}
	return s.ProbeLocation(ctx, location.ID())
}

func (s *Service) CodexAuthStatus(ctx context.Context, locationID locationdomain.ID) (CodexAuthStatusResult, error) {
	if s == nil {
		return CodexAuthStatusResult{}, fmt.Errorf("location service is nil")
	}
	location, err := s.GetLocation(ctx, locationID)
	if err != nil {
		return CodexAuthStatusResult{}, err
	}
	return s.transport.CodexAuthStatus(ctx, location)
}

func (s *Service) BeginCodexLogin(ctx context.Context, locationID locationdomain.ID) (CodexLoginResult, error) {
	if s == nil {
		return CodexLoginResult{}, fmt.Errorf("location service is nil")
	}
	location, err := s.GetLocation(ctx, locationID)
	if err != nil {
		return CodexLoginResult{}, err
	}
	return s.transport.BeginCodexLogin(ctx, location)
}

func (s *Service) InstallClaude(ctx context.Context, locationID locationdomain.ID) (locationdomain.Location, error) {
	if s == nil {
		return locationdomain.Location{}, fmt.Errorf("location service is nil")
	}
	location, err := s.GetLocation(ctx, locationID)
	if err != nil {
		return locationdomain.Location{}, err
	}
	if _, err := s.transport.InstallClaude(ctx, location); err != nil {
		return locationdomain.Location{}, err
	}
	return s.ProbeLocation(ctx, location.ID())
}

func (s *Service) ClaudeAuthStatus(ctx context.Context, locationID locationdomain.ID) (ClaudeAuthStatusResult, error) {
	if s == nil {
		return ClaudeAuthStatusResult{}, fmt.Errorf("location service is nil")
	}
	location, err := s.GetLocation(ctx, locationID)
	if err != nil {
		return ClaudeAuthStatusResult{}, err
	}
	return s.transport.ClaudeAuthStatus(ctx, location)
}

func (s *Service) BeginClaudeLogin(ctx context.Context, locationID locationdomain.ID) (ClaudeLoginResult, error) {
	if s == nil {
		return ClaudeLoginResult{}, fmt.Errorf("location service is nil")
	}
	location, err := s.GetLocation(ctx, locationID)
	if err != nil {
		return ClaudeLoginResult{}, err
	}
	return s.transport.BeginClaudeLogin(ctx, location)
}

func (s *Service) CompleteClaudeLogin(ctx context.Context, locationID locationdomain.ID, code string) (ClaudeLoginResult, error) {
	if s == nil {
		return ClaudeLoginResult{}, fmt.Errorf("location service is nil")
	}
	code = strings.TrimSpace(code)
	if code == "" {
		return ClaudeLoginResult{}, fmt.Errorf("authorization code is required")
	}
	location, err := s.GetLocation(ctx, locationID)
	if err != nil {
		return ClaudeLoginResult{}, err
	}
	return s.transport.CompleteClaudeLogin(ctx, location, code)
}

func (s *Service) StartCommand(ctx context.Context, locationID locationdomain.ID, spec CommandSpec) (CommandProcess, error) {
	if s == nil {
		return nil, fmt.Errorf("location service is nil")
	}
	location, err := s.GetLocation(ctx, locationID)
	if err != nil {
		return nil, err
	}
	if !location.Ready() {
		return nil, fmt.Errorf("location %q is not ready", location.ID())
	}
	if strings.TrimSpace(spec.Command) == "" {
		return nil, fmt.Errorf("command is required")
	}
	return s.transport.StartCommand(ctx, location, spec)
}

func (s *Service) EnsureBridge(ctx context.Context, locationID locationdomain.ID) (Bridge, error) {
	if s == nil {
		return Bridge{}, fmt.Errorf("location service is nil")
	}
	started := time.Now()
	s.logger.InfoContext(ctx, "location bridge ensure starting", "location_id", locationID)
	unlock := s.lockBridge(locationID)
	defer unlock()
	location, err := s.GetLocation(ctx, locationID)
	if err != nil {
		s.logger.ErrorContext(ctx, "location bridge ensure load failed", "location_id", locationID, "error", err)
		return Bridge{}, err
	}
	if !location.Ready() {
		s.logger.WarnContext(ctx, "location bridge ensure rejected not ready", "location_id", location.ID(), "kind", location.Kind(), "status", location.Status())
		return Bridge{}, fmt.Errorf("location %q is not ready", location.ID())
	}
	address := location.Address()
	s.logger.InfoContext(ctx, "location bridge transport ensure starting",
		"location_id", location.ID(),
		"kind", location.Kind(),
		"host", address.Host,
		"port", address.Port,
		"username", address.Username,
	)
	host, err := s.transport.EnsureBridge(ctx, location)
	if err != nil {
		s.logger.ErrorContext(ctx, "location bridge transport ensure failed",
			"location_id", location.ID(),
			"kind", location.Kind(),
			"host", address.Host,
			"port", address.Port,
			"error", err,
			"duration_ms", time.Since(started).Milliseconds(),
		)
		return Bridge{}, err
	}
	if strings.TrimSpace(host.WebSocketURL) == "" {
		s.logger.ErrorContext(ctx, "location bridge returned no websocket url", "location_id", location.ID(), "kind", location.Kind())
		return Bridge{}, fmt.Errorf("location %q returned no bridge websocket URL", location.ID())
	}
	s.logger.InfoContext(ctx, "location bridge ensure ready",
		"location_id", location.ID(),
		"kind", location.Kind(),
		"has_websocket_url", strings.TrimSpace(host.WebSocketURL) != "",
		"has_mcp_base_url", strings.TrimSpace(host.MCPBaseURL) != "",
		"diagnostics", strings.TrimSpace(host.Diagnostics),
		"duration_ms", time.Since(started).Milliseconds(),
	)
	return host, nil
}

func (s *Service) lockBridge(locationID locationdomain.ID) func() {
	s.bridgeMu.Lock()
	lock := s.bridges[locationID]
	if lock == nil {
		lock = &sync.Mutex{}
		s.bridges[locationID] = lock
	}
	s.bridgeMu.Unlock()

	lock.Lock()
	return lock.Unlock
}

func (s *Service) now() time.Time {
	if s == nil || s.clock == nil {
		return time.Now().UTC()
	}
	return s.clock.Now().UTC()
}

func (s *Service) ensureLocalHost(ctx context.Context, location locationdomain.Location) (locationdomain.Location, error) {
	record := location.Record()
	if record.Kind != locationdomain.KindLocal {
		return location, nil
	}
	host := s.localAddress().Host
	if host == "" || record.Address.Host == host {
		return location, nil
	}
	record.Address.Host = host
	record.UpdatedAt = s.now()
	saved, err := s.locations.Save(ctx, record)
	if err != nil {
		return locationdomain.Location{}, err
	}
	return locationdomain.Wrap(saved)
}

func (s *Service) localAddress() locationdomain.Address {
	if s == nil || s.hostname == nil {
		return locationdomain.Address{}
	}
	host, err := s.hostname()
	if err != nil {
		return locationdomain.Address{}
	}
	return locationdomain.Address{Host: strings.TrimSpace(host)}
}

func locationIDForInput(kind locationdomain.Kind, label string, address locationdomain.Address) locationdomain.ID {
	raw := strings.Join([]string{
		string(kind),
		strings.TrimSpace(label),
		strings.TrimSpace(address.Username),
		strings.TrimSpace(address.Host),
		fmt.Sprintf("%d", address.Port),
	}, "|")
	sum := sha1.Sum([]byte(raw))
	return locationdomain.ID("loc_" + hex.EncodeToString(sum[:])[:16])
}
