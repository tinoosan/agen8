package builtins

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/playwright-community/playwright-go"
)

const (
	DefaultBrowserMaxSessions = 5
)

type browserSession struct {
	id        string
	browser   playwright.Browser
	context   playwright.BrowserContext
	page      playwright.Page
	createdAt time.Time
	lastUsed  time.Time

	opMu   sync.Mutex
	closed bool
}

// BrowserSessionManager manages long-lived Playwright browser sessions.
//
// This type intentionally does not require Playwright to be installed/initialized until
// the first call to Start(). That keeps the feature additive: runtimes can enable the
// tool without failing startup on hosts that haven't installed browser binaries yet.
type BrowserSessionManager struct {
	mu       sync.Mutex
	sessions map[string]*browserSession

	timeout     time.Duration
	maxSessions int

	pwOnce sync.Once
	pw     *playwright.Playwright
	pwErr  error
	// runOptions keep a shared RunOptions pointer so install and run use the same cache.
	runOptions *playwright.RunOptions
	// installOnce ensures we only attempt to download browsers one time.
	installOnce sync.Once
	installErr  error
	installFunc func() error
}

func NewBrowserSessionManager(timeout time.Duration) (*BrowserSessionManager, error) {
	if timeout <= 0 {
		return nil, fmt.Errorf("timeout must be > 0")
	}
	runOpts := &playwright.RunOptions{
		Browsers: []string{"chromium"},
		Verbose:  false,
		Stdout:   io.Discard,
		Stderr:   io.Discard,
	}
	m := &BrowserSessionManager{
		sessions:    make(map[string]*browserSession),
		timeout:     timeout,
		maxSessions: DefaultBrowserMaxSessions,
		runOptions:  runOpts,
	}
	m.installFunc = func() error {
		return playwright.Install(m.runOptions)
	}
	return m, nil
}

func (m *BrowserSessionManager) ensurePlaywright() error {
	if m == nil {
		return fmt.Errorf("browser manager is nil")
	}
	if err := m.installBrowsers(); err != nil {
		return err
	}
	opts := m.runOptions
	if opts == nil {
		opts = &playwright.RunOptions{
			Browsers: []string{"chromium"},
			Verbose:  false,
			Stdout:   io.Discard,
			Stderr:   io.Discard,
		}
	}
	m.pwOnce.Do(func() {
		m.pw, m.pwErr = playwright.Run(opts)
	})
	return m.pwErr
}

func (m *BrowserSessionManager) installBrowsers() error {
	if m == nil {
		return fmt.Errorf("browser manager is nil")
	}
	if m.installFunc == nil {
		return fmt.Errorf("browser install function is not configured")
	}
	m.installOnce.Do(func() {
		m.installErr = m.installFunc()
	})
	if m.installErr != nil {
		return fmt.Errorf("install playwright browsers: %w", m.installErr)
	}
	return nil
}

func (m *BrowserSessionManager) SetMaxSessions(n int) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.maxSessions = n
}

func (m *BrowserSessionManager) Start(ctx context.Context, headless bool) (string, error) {
	_ = ctx
	if err := m.ensurePlaywright(); err != nil {
		return "", fmt.Errorf("start playwright: %w", err)
	}

	m.mu.Lock()
	maxSessions := m.maxSessions
	active := len(m.sessions)
	m.mu.Unlock()

	if maxSessions > 0 && active >= maxSessions {
		return "", fmt.Errorf("max browser sessions reached (%d)", maxSessions)
	}

	browser, err := m.pw.Chromium.Launch(playwright.BrowserTypeLaunchOptions{
		Headless: playwright.Bool(headless),
	})
	if err != nil {
		return "", fmt.Errorf("launch chromium: %w", err)
	}

	ctxx, err := browser.NewContext()
	if err != nil {
		_ = browser.Close()
		return "", fmt.Errorf("new browser context: %w", err)
	}

	page, err := ctxx.NewPage()
	if err != nil {
		_ = ctxx.Close()
		_ = browser.Close()
		return "", fmt.Errorf("new page: %w", err)
	}

	now := time.Now()
	s := &browserSession{
		id:        uuid.NewString(),
		browser:   browser,
		context:   ctxx,
		page:      page,
		createdAt: now,
		lastUsed:  now,
	}

	m.mu.Lock()
	m.sessions[s.id] = s
	m.mu.Unlock()

	return s.id, nil
}

func (m *BrowserSessionManager) Navigate(ctx context.Context, sessionID, url, waitFor string) (title string, finalURL string, err error) {
	err = m.withSession(ctx, sessionID, func(s *browserSession) error {
		if strings.TrimSpace(url) == "" {
			return fmt.Errorf("url is required")
		}
		if _, err := s.page.Goto(strings.TrimSpace(url)); err != nil {
			return err
		}
		waitFor = strings.TrimSpace(waitFor)
		if waitFor != "" {
			if _, err := s.page.WaitForSelector(waitFor); err != nil {
				return err
			}
		}
		t, _ := s.page.Title()
		title = strings.TrimSpace(t)
		finalURL = strings.TrimSpace(s.page.URL())
		return nil
	})
	return
}

func (m *BrowserSessionManager) Click(ctx context.Context, sessionID, selector, waitFor string) error {
	return m.withSession(ctx, sessionID, func(s *browserSession) error {
		selector = strings.TrimSpace(selector)
		if selector == "" {
			return fmt.Errorf("selector is required")
		}
		if err := s.page.Click(selector); err != nil {
			return err
		}
		waitFor = strings.TrimSpace(waitFor)
		if waitFor != "" {
			_, _ = s.page.WaitForSelector(waitFor)
		}
		return nil
	})
}

func (m *BrowserSessionManager) Fill(ctx context.Context, sessionID, selector, text, waitFor string) error {
	return m.withSession(ctx, sessionID, func(s *browserSession) error {
		selector = strings.TrimSpace(selector)
		if selector == "" {
			return fmt.Errorf("selector is required")
		}
		if err := s.page.Fill(selector, text); err != nil {
			return err
		}
		waitFor = strings.TrimSpace(waitFor)
		if waitFor != "" {
			_, _ = s.page.WaitForSelector(waitFor)
		}
		return nil
	})
}

func (m *BrowserSessionManager) Extract(ctx context.Context, sessionID, selector, attribute string) (json.RawMessage, error) {
	var out json.RawMessage
	err := m.withSession(ctx, sessionID, func(s *browserSession) error {
		selector = strings.TrimSpace(selector)
		if selector == "" {
			return fmt.Errorf("selector is required")
		}
		attribute = strings.TrimSpace(attribute)
		if attribute == "" {
			attribute = "textContent"
		}

		expr := selectorAllExpr(attribute)
		result, err := s.page.EvalOnSelectorAll(selector, expr)
		if err != nil {
			return err
		}
		b, err := json.Marshal(result)
		if err != nil {
			return err
		}
		out = b
		return nil
	})
	return out, err
}

func (m *BrowserSessionManager) Screenshot(ctx context.Context, sessionID, absPath string, fullPage bool) error {
	_ = ctx
	return m.withSession(ctx, sessionID, func(s *browserSession) error {
		if strings.TrimSpace(absPath) == "" || !filepath.IsAbs(absPath) {
			return fmt.Errorf("absPath must be an absolute path")
		}
		if err := os.MkdirAll(filepath.Dir(absPath), 0755); err != nil {
			return err
		}
		_, err := s.page.Screenshot(playwright.PageScreenshotOptions{
			Path:     playwright.String(absPath),
			FullPage: playwright.Bool(fullPage),
		})
		return err
	})
}

func (m *BrowserSessionManager) PDF(ctx context.Context, sessionID, absPath string) error {
	_ = ctx
	return m.withSession(ctx, sessionID, func(s *browserSession) error {
		if strings.TrimSpace(absPath) == "" || !filepath.IsAbs(absPath) {
			return fmt.Errorf("absPath must be an absolute path")
		}
		if err := os.MkdirAll(filepath.Dir(absPath), 0755); err != nil {
			return err
		}
		_, err := s.page.PDF(playwright.PagePdfOptions{
			Path: playwright.String(absPath),
		})
		return err
	})
}

func (m *BrowserSessionManager) Close(ctx context.Context, sessionID string) error {
	_ = ctx
	s, err := m.takeSession(sessionID)
	if err != nil {
		return err
	}
	return closeSession(s)
}

func (m *BrowserSessionManager) CleanupStale() {
	if m == nil {
		return
	}

	var stale []*browserSession
	now := time.Now()

	m.mu.Lock()
	for id, s := range m.sessions {
		if m.timeout > 0 && now.Sub(s.lastUsed) > m.timeout {
			delete(m.sessions, id)
			stale = append(stale, s)
		}
	}
	m.mu.Unlock()

	for _, s := range stale {
		_ = closeSession(s)
	}
}

func (m *BrowserSessionManager) Shutdown() error {
	if m == nil {
		return nil
	}

	m.mu.Lock()
	sessions := make([]*browserSession, 0, len(m.sessions))
	for _, s := range m.sessions {
		sessions = append(sessions, s)
	}
	m.sessions = make(map[string]*browserSession)
	pw := m.pw
	pwErr := m.pwErr
	m.mu.Unlock()

	for _, s := range sessions {
		_ = closeSession(s)
	}

	// If Playwright never initialized, nothing to stop.
	if pw == nil || pwErr != nil {
		return nil
	}
	return pw.Stop()
}

func (m *BrowserSessionManager) takeSession(sessionID string) (*browserSession, error) {
	if m == nil {
		return nil, fmt.Errorf("browser manager is nil")
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, fmt.Errorf("sessionId is required")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sessions[sessionID]
	if !ok {
		return nil, fmt.Errorf("session not found: %s", sessionID)
	}
	delete(m.sessions, sessionID)
	return s, nil
}

func (m *BrowserSessionManager) withSession(_ context.Context, sessionID string, fn func(*browserSession) error) error {
	if m == nil {
		return fmt.Errorf("browser manager is nil")
	}
	if err := m.ensurePlaywright(); err != nil {
		return fmt.Errorf("start playwright: %w", err)
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return fmt.Errorf("sessionId is required")
	}

	m.mu.Lock()
	s, ok := m.sessions[sessionID]
	if !ok {
		m.mu.Unlock()
		return fmt.Errorf("session not found: %s", sessionID)
	}
	s.lastUsed = time.Now()
	m.mu.Unlock()

	s.opMu.Lock()
	defer s.opMu.Unlock()
	if s.closed {
		return fmt.Errorf("session closed: %s", sessionID)
	}
	if err := fn(s); err != nil {
		return err
	}

	m.mu.Lock()
	if cur, ok := m.sessions[sessionID]; ok && cur == s {
		s.lastUsed = time.Now()
	}
	m.mu.Unlock()
	return nil
}

func closeSession(s *browserSession) error {
	if s == nil {
		return nil
	}

	s.opMu.Lock()
	if s.closed {
		s.opMu.Unlock()
		return nil
	}
	s.closed = true
	page := s.page
	ctxx := s.context
	browser := s.browser
	s.opMu.Unlock()

	// Best-effort close order.
	if page != nil {
		_ = page.Close()
	}
	if ctxx != nil {
		_ = ctxx.Close()
	}
	if browser != nil {
		_ = browser.Close()
	}
	return nil
}

func selectorAllExpr(attr string) string {
	switch strings.TrimSpace(attr) {
	case "", "textContent":
		return `(elements) => elements.map(el => el.textContent)`
	case "innerText":
		return `(elements) => elements.map(el => el.innerText)`
	case "innerHTML":
		return `(elements) => elements.map(el => el.innerHTML)`
	case "outerHTML":
		return `(elements) => elements.map(el => el.outerHTML)`
	case "href":
		return `(elements) => elements.map(el => el.href)`
	case "src":
		return `(elements) => elements.map(el => el.src)`
	case "value":
		return `(elements) => elements.map(el => el.value)`
	default:
		// Safe string interpolation: embed as JSON string literal.
		b, _ := json.Marshal(attr)
		return fmt.Sprintf(`(elements) => elements.map(el => el.getAttribute(%s))`, string(b))
	}
}
