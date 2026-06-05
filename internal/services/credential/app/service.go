package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	credentialdomain "github.com/tinoosan/agen8-mcp-server/internal/services/credential/domain"
)

type Service struct {
	repository credentialdomain.Repository
	clock      credentialdomain.Clock
}

type Config struct {
	Repository credentialdomain.Repository
	Clock      credentialdomain.Clock
}

func NewService(config Config) (*Service, error) {
	if config.Repository == nil {
		return nil, fmt.Errorf("credential repository is required")
	}
	clock := config.Clock
	if clock == nil {
		clock = credentialdomain.SystemClock{}
	}
	return &Service{
		repository: config.Repository,
		clock:      clock,
	}, nil
}

type CreateCredentialInput struct {
	Kind        credentialdomain.Kind
	Label       string
	StorageKind credentialdomain.StorageKind
	Secrets     map[string]string
}

type UpdateCredentialInput struct {
	ID          credentialdomain.ID
	Label       string
	Status      credentialdomain.Status
	StorageKind credentialdomain.StorageKind
	Secrets     map[string]string
}

type ResolveCredentialInput struct {
	CredentialID credentialdomain.ID
	Purpose      credentialdomain.Purpose
}

func (s *Service) ListCredentials(ctx context.Context, filter credentialdomain.Filter) ([]credentialdomain.Credential, error) {
	if s == nil {
		return nil, fmt.Errorf("credential service is nil")
	}
	records, err := s.repository.List(ctx, filter)
	if err != nil {
		return nil, err
	}
	out := make([]credentialdomain.Credential, 0, len(records))
	for _, record := range records {
		credential, err := credentialdomain.Wrap(record)
		if err != nil {
			return nil, err
		}
		out = append(out, credential)
	}
	return out, nil
}

func (s *Service) GetCredential(ctx context.Context, id credentialdomain.ID) (credentialdomain.Credential, error) {
	if s == nil {
		return credentialdomain.Credential{}, fmt.Errorf("credential service is nil")
	}
	id = credentialdomain.ID(strings.TrimSpace(string(id)))
	if id == "" {
		return credentialdomain.Credential{}, fmt.Errorf("credential id is required")
	}
	record, err := s.repository.Get(ctx, id)
	if err != nil {
		return credentialdomain.Credential{}, err
	}
	return credentialdomain.Wrap(record)
}

func (s *Service) CreateCredential(ctx context.Context, input CreateCredentialInput) (credentialdomain.Credential, error) {
	if s == nil {
		return credentialdomain.Credential{}, fmt.Errorf("credential service is nil")
	}
	kind := credentialdomain.Kind(strings.TrimSpace(string(input.Kind)))
	storageKind := cleanStorageKind(input.StorageKind, kind)
	secrets := cleanSecrets(input.Secrets)
	secrets = normalizeAPIKeySecrets(secrets)
	if err := validateCreate(kind, storageKind, secrets); err != nil {
		return credentialdomain.Credential{}, err
	}
	now := s.now()
	record := credentialdomain.Record{
		ID:        newCredentialID(),
		Kind:      kind,
		Label:     strings.TrimSpace(input.Label),
		Status:    credentialdomain.StatusActive,
		Fields:    fieldsForSecrets(secrets, kind),
		CreatedAt: now,
		UpdatedAt: now,
	}
	saved, err := s.repository.Save(ctx, record)
	if err != nil {
		return credentialdomain.Credential{}, err
	}
	if storageKind != credentialdomain.StorageSSHAgent {
		payload, err := marshalSecrets(secrets)
		if err != nil {
			return credentialdomain.Credential{}, err
		}
		if err := s.repository.PutMaterial(ctx, credentialdomain.SecretMaterial{
			CredentialID: saved.ID,
			StorageKind:  storageKind,
			Payload:      payload,
			UpdatedAt:    now,
		}); err != nil {
			return credentialdomain.Credential{}, err
		}
	} else if err := s.repository.PutMaterial(ctx, credentialdomain.SecretMaterial{
		CredentialID: saved.ID,
		StorageKind:  credentialdomain.StorageSSHAgent,
		UpdatedAt:    now,
	}); err != nil {
		return credentialdomain.Credential{}, err
	}
	return credentialdomain.Wrap(saved)
}

func (s *Service) UpdateCredential(ctx context.Context, input UpdateCredentialInput) (credentialdomain.Credential, error) {
	if s == nil {
		return credentialdomain.Credential{}, fmt.Errorf("credential service is nil")
	}
	id := credentialdomain.ID(strings.TrimSpace(string(input.ID)))
	if id == "" {
		return credentialdomain.Credential{}, fmt.Errorf("credential id is required")
	}
	current, err := s.repository.Get(ctx, id)
	if err != nil {
		return credentialdomain.Credential{}, err
	}
	now := s.now()
	if strings.TrimSpace(input.Label) != "" {
		current.Label = strings.TrimSpace(input.Label)
	}
	if input.Status != "" {
		current.Status = credentialdomain.Status(strings.TrimSpace(string(input.Status)))
	}
	secrets := cleanSecrets(input.Secrets)
	if len(input.Secrets) > 0 {
		secrets = normalizeAPIKeySecrets(secrets)
		storageKind := cleanStorageKind(input.StorageKind, current.Kind)
		if err := validateCreate(current.Kind, storageKind, secrets); err != nil {
			return credentialdomain.Credential{}, err
		}
		current.Fields = fieldsForSecrets(secrets, current.Kind)
		payload, err := marshalSecrets(secrets)
		if err != nil {
			return credentialdomain.Credential{}, err
		}
		if storageKind == credentialdomain.StorageSSHAgent {
			payload = nil
		}
		if err := s.repository.PutMaterial(ctx, credentialdomain.SecretMaterial{
			CredentialID: current.ID,
			StorageKind:  storageKind,
			Payload:      payload,
			UpdatedAt:    now,
		}); err != nil {
			return credentialdomain.Credential{}, err
		}
	}
	current.UpdatedAt = now
	saved, err := s.repository.Save(ctx, current)
	if err != nil {
		return credentialdomain.Credential{}, err
	}
	return credentialdomain.Wrap(saved)
}

func (s *Service) DeleteCredential(ctx context.Context, id credentialdomain.ID) error {
	if s == nil {
		return fmt.Errorf("credential service is nil")
	}
	id = credentialdomain.ID(strings.TrimSpace(string(id)))
	if id == "" {
		return fmt.Errorf("credential id is required")
	}
	if err := s.repository.DeleteMaterial(ctx, id); err != nil && !errors.Is(err, credentialdomain.ErrNotFound) {
		return err
	}
	return s.repository.Delete(ctx, id)
}

func (s *Service) ResolveCredential(ctx context.Context, input ResolveCredentialInput) (credentialdomain.ResolvedCredential, error) {
	if s == nil {
		return credentialdomain.ResolvedCredential{}, fmt.Errorf("credential service is nil")
	}
	id := credentialdomain.ID(strings.TrimSpace(string(input.CredentialID)))
	if id == "" {
		return credentialdomain.ResolvedCredential{}, fmt.Errorf("credential id is required")
	}
	purpose := credentialdomain.Purpose(strings.TrimSpace(string(input.Purpose)))
	if purpose == "" {
		return credentialdomain.ResolvedCredential{}, fmt.Errorf("credential purpose is required")
	}
	record, err := s.repository.Get(ctx, id)
	if err != nil {
		return credentialdomain.ResolvedCredential{}, err
	}
	if record.Status != credentialdomain.StatusActive {
		return credentialdomain.ResolvedCredential{}, fmt.Errorf("credential %s is not active", id)
	}
	if err := validatePurpose(record.Kind, purpose); err != nil {
		return credentialdomain.ResolvedCredential{}, err
	}
	if record.Kind == credentialdomain.KindSSHAgent {
		return credentialdomain.ResolvedCredential{ID: id, Kind: record.Kind, Purpose: purpose, Values: map[string]string{}}, nil
	}
	material, err := s.repository.GetMaterial(ctx, id)
	if err != nil {
		return credentialdomain.ResolvedCredential{}, err
	}
	values, err := unmarshalSecrets(material.Payload)
	if err != nil {
		return credentialdomain.ResolvedCredential{}, err
	}
	return credentialdomain.ResolvedCredential{
		ID:      id,
		Kind:    record.Kind,
		Purpose: purpose,
		Values:  values,
	}, nil
}

func (s *Service) now() time.Time {
	if s.clock == nil {
		return credentialdomain.SystemClock{}.Now()
	}
	return s.clock.Now().UTC()
}

func cleanStorageKind(storageKind credentialdomain.StorageKind, kind credentialdomain.Kind) credentialdomain.StorageKind {
	storageKind = credentialdomain.StorageKind(strings.TrimSpace(string(storageKind)))
	if storageKind != "" {
		return storageKind
	}
	if kind == credentialdomain.KindSSHAgent {
		return credentialdomain.StorageSSHAgent
	}
	return credentialdomain.StorageLocalEncrypted
}

func validateCreate(kind credentialdomain.Kind, storageKind credentialdomain.StorageKind, secrets map[string]string) error {
	switch kind {
	case credentialdomain.KindSSHAgent:
		if storageKind != credentialdomain.StorageSSHAgent {
			return fmt.Errorf("ssh_agent credentials require ssh_agent storage")
		}
		return nil
	case credentialdomain.KindSSHKey:
		if storageKind != credentialdomain.StorageLocalEncrypted {
			return fmt.Errorf("ssh_key credentials require local_encrypted storage")
		}
		if strings.TrimSpace(secrets["privateKey"]) == "" {
			return fmt.Errorf("ssh_key credentials require privateKey")
		}
		return nil
	case credentialdomain.KindSSHPassword:
		if storageKind != credentialdomain.StorageLocalEncrypted {
			return fmt.Errorf("ssh_password credentials require local_encrypted storage")
		}
		if strings.TrimSpace(secrets["password"]) == "" {
			return fmt.Errorf("ssh_password credentials require password")
		}
		return nil
	case credentialdomain.KindAPIKey:
		if storageKind != credentialdomain.StorageLocalEncrypted {
			return fmt.Errorf("api_key credentials require local_encrypted storage")
		}
		if strings.TrimSpace(secrets["value"]) == "" {
			return fmt.Errorf("api_key credentials require value")
		}
		if err := validateAPIKeyHTTPConfig(secrets); err != nil {
			return err
		}
		return nil
	default:
		return fmt.Errorf("unsupported credential kind %q", kind)
	}
}

func validatePurpose(kind credentialdomain.Kind, purpose credentialdomain.Purpose) error {
	switch purpose {
	case credentialdomain.PurposeLocationSSH:
		if kind == credentialdomain.KindSSHAgent || kind == credentialdomain.KindSSHKey || kind == credentialdomain.KindSSHPassword {
			return nil
		}
	case credentialdomain.PurposeHTTPTool:
		if kind == credentialdomain.KindAPIKey {
			return nil
		}
	default:
		return fmt.Errorf("unsupported credential purpose %q", purpose)
	}
	return fmt.Errorf("credential kind %q cannot resolve for purpose %q", kind, purpose)
}

func cleanSecrets(secrets map[string]string) map[string]string {
	out := make(map[string]string, len(secrets))
	for key, value := range secrets {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		out[key] = strings.TrimSpace(value)
	}
	return out
}

func normalizeAPIKeySecrets(secrets map[string]string) map[string]string {
	if len(secrets) == 0 {
		return secrets
	}
	out := make(map[string]string, len(secrets))
	for key, value := range secrets {
		out[key] = value
	}
	if headerName := strings.TrimSpace(out["headerName"]); headerName != "" {
		out["headerName"] = canonicalHTTPHeaderName(headerName)
	}
	return out
}

func validateAPIKeyHTTPConfig(secrets map[string]string) error {
	injection := strings.ToLower(strings.TrimSpace(secrets["injection"]))
	headerName := strings.TrimSpace(secrets["headerName"])
	paramName := strings.TrimSpace(secrets["paramName"])
	switch injection {
	case "", string(credentialdomain.InjectionBearer):
		return nil
	case string(credentialdomain.InjectionHeader):
		if headerName == "" {
			return fmt.Errorf("api_key header credentials require headerName")
		}
		if !validHTTPHeaderName(headerName) {
			return fmt.Errorf("api_key headerName %q is not a valid HTTP header name", headerName)
		}
		return nil
	case string(credentialdomain.InjectionQuery):
		if paramName == "" {
			return fmt.Errorf("api_key query credentials require paramName")
		}
		if strings.ContainsAny(paramName, "=&?#") {
			return fmt.Errorf("api_key paramName %q is not a valid query parameter name", paramName)
		}
		return nil
	default:
		return fmt.Errorf("api_key injection must be bearer, header, or query")
	}
}

func canonicalHTTPHeaderName(name string) string {
	name = strings.TrimSpace(name)
	switch strings.ToUpper(strings.NewReplacer("-", "_", " ", "_").Replace(name)) {
	case "ALPACA_PAPER_API_KEY", "ALPACA_API_KEY", "ALPACA_API_KEY_ID", "APCA_API_KEY_ID":
		return "APCA-API-KEY-ID"
	case "ALPACA_PAPER_SECRET_KEY", "ALPACA_SECRET_KEY", "ALPACA_API_SECRET_KEY", "APCA_API_SECRET_KEY", "APCA_PAPER_SECRET_KEY":
		return "APCA-API-SECRET-KEY"
	default:
		return name
	}
}

func validHTTPHeaderName(name string) bool {
	if strings.TrimSpace(name) != name || name == "" {
		return false
	}
	for _, r := range name {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' {
			continue
		}
		switch r {
		case '!', '#', '$', '%', '&', '\'', '*', '+', '-', '.', '^', '_', '`', '|', '~':
			continue
		default:
			return false
		}
	}
	return true
}

func fieldsForSecrets(secrets map[string]string, kind credentialdomain.Kind) []credentialdomain.FieldRef {
	if kind == credentialdomain.KindSSHAgent {
		return []credentialdomain.FieldRef{{Name: "agent", Kind: credentialdomain.FieldPublic}}
	}
	fields := make([]credentialdomain.FieldRef, 0, len(secrets))
	for name := range secrets {
		fields = append(fields, credentialdomain.FieldRef{Name: name, Kind: fieldKind(name)})
	}
	sort.Slice(fields, func(i, j int) bool {
		return fields[i].Name < fields[j].Name
	})
	return fields
}

func fieldKind(name string) credentialdomain.FieldKind {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "paramname", "headername", "name", "username", "user", "agent":
		return credentialdomain.FieldPublic
	default:
		return credentialdomain.FieldSecret
	}
}

func marshalSecrets(secrets map[string]string) ([]byte, error) {
	raw, err := json.Marshal(secrets)
	if err != nil {
		return nil, fmt.Errorf("marshal credential secrets: %w", err)
	}
	return raw, nil
}

func unmarshalSecrets(raw []byte) (map[string]string, error) {
	var values map[string]string
	if err := json.Unmarshal(raw, &values); err != nil {
		return nil, fmt.Errorf("unmarshal credential secrets: %w", err)
	}
	if values == nil {
		values = map[string]string{}
	}
	return values, nil
}

func newCredentialID() credentialdomain.ID {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(fmt.Sprintf("credential id entropy unavailable: %v", err))
	}
	return credentialdomain.ID("cred_" + hex.EncodeToString(b[:]))
}
