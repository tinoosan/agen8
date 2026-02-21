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
	defaultBrowserTimeoutMs   = 2 * 60 * 1000
)

type browserSession struct {
	id        string
	browser   playwright.Browser
	context   playwright.BrowserContext
	pages     map[string]playwright.Page
	activeID  string
	createdAt time.Time
	lastUsed  time.Time

	userAgent    string
	viewportW    int
	viewportH    int
	extraHeaders map[string]string

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

func (m *BrowserSessionManager) Start(ctx context.Context, headless bool, userAgent string, viewportWidth, viewportHeight int, extraHeaders map[string]string) (string, error) {
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

	userAgent = strings.TrimSpace(userAgent)
	var uaPtr *string
	if userAgent != "" {
		uaPtr = playwright.String(userAgent)
	}
	var viewport *playwright.Size
	if viewportWidth > 0 && viewportHeight > 0 {
		viewport = &playwright.Size{Width: viewportWidth, Height: viewportHeight}
	}
	var headers map[string]string
	if len(extraHeaders) != 0 {
		headers = make(map[string]string, len(extraHeaders))
		for k, v := range extraHeaders {
			k = strings.TrimSpace(k)
			if k == "" {
				continue
			}
			headers[k] = v
		}
	}

	ctxx, err := browser.NewContext(playwright.BrowserNewContextOptions{
		AcceptDownloads:  playwright.Bool(true),
		UserAgent:        uaPtr,
		Viewport:         viewport,
		ExtraHttpHeaders: headers,
	})
	if err != nil {
		_ = browser.Close()
		return "", fmt.Errorf("new browser context: %w", err)
	}

	// Default Playwright timeout is 30s; for human-like browsing and JS-heavy pages,
	// use a larger default.
	ctxx.SetDefaultTimeout(defaultBrowserTimeoutMs)
	ctxx.SetDefaultNavigationTimeout(defaultBrowserTimeoutMs)

	page, err := ctxx.NewPage()
	if err != nil {
		_ = ctxx.Close()
		_ = browser.Close()
		return "", fmt.Errorf("new page: %w", err)
	}
	page.SetDefaultTimeout(defaultBrowserTimeoutMs)
	page.SetDefaultNavigationTimeout(defaultBrowserTimeoutMs)

	now := time.Now()
	s := &browserSession{
		id:           uuid.NewString(),
		browser:      browser,
		context:      ctxx,
		pages:        map[string]playwright.Page{},
		createdAt:    now,
		lastUsed:     now,
		userAgent:    userAgent,
		viewportW:    viewportWidth,
		viewportH:    viewportHeight,
		extraHeaders: headers,
	}
	pageID := uuid.NewString()[:8]
	s.pages[pageID] = page
	s.activeID = pageID

	m.mu.Lock()
	m.sessions[s.id] = s
	m.mu.Unlock()

	return s.id, nil
}

func (m *BrowserSessionManager) Navigate(ctx context.Context, sessionID, url, waitFor string, timeoutMs int) (title string, finalURL string, err error) {
	err = m.withActivePage(ctx, sessionID, func(s *browserSession, page playwright.Page) error {
		_ = s
		if strings.TrimSpace(url) == "" {
			return fmt.Errorf("url is required")
		}
		opts := playwright.PageGotoOptions{
			// Prefer DOMContentLoaded so we can interact with the page sooner; many sites
			// never reach "load" due to long-polling and ads.
			WaitUntil: playwright.WaitUntilStateDomcontentloaded,
		}
		if timeoutMs > 0 {
			opts.Timeout = playwright.Float(float64(timeoutMs))
		}
		if _, err := page.Goto(strings.TrimSpace(url), opts); err != nil {
			return err
		}
		waitFor = strings.TrimSpace(waitFor)
		if waitFor != "" {
			loc := page.Locator(waitFor).First()
			if err := loc.WaitFor(); err != nil {
				return err
			}
		}
		t, _ := page.Title()
		title = strings.TrimSpace(t)
		finalURL = strings.TrimSpace(page.URL())
		return nil
	})
	return
}

func (m *BrowserSessionManager) Click(ctx context.Context, sessionID, selector, waitFor string, expectPopup bool, timeoutMs int) (popupPageID, popupTitle, popupURL string, err error) {
	err = m.withActivePage(ctx, sessionID, func(s *browserSession, page playwright.Page) error {
		selector = strings.TrimSpace(selector)
		if selector == "" {
			return fmt.Errorf("selector is required")
		}
		waitFor = strings.TrimSpace(waitFor)

		clickFn := func() error {
			loc := page.Locator(selector).First()
			opts := playwright.LocatorClickOptions{
				NoWaitAfter: playwright.Bool(true),
			}
			if timeoutMs > 0 {
				opts.Timeout = playwright.Float(float64(timeoutMs))
			}
			return loc.Click(opts)
		}

		if expectPopup {
			opts := playwright.PageExpectPopupOptions{}
			if timeoutMs > 0 {
				opts.Timeout = playwright.Float(float64(timeoutMs))
			}
			popup, err := page.ExpectPopup(clickFn, opts)
			if err != nil {
				return err
			}
			id := uuid.NewString()[:8]
			if s.pages == nil {
				s.pages = make(map[string]playwright.Page)
			}
			s.pages[id] = popup
			s.activeID = id
			configurePageDefaults(popup)
			if s.viewportW > 0 && s.viewportH > 0 {
				_ = popup.SetViewportSize(s.viewportW, s.viewportH)
			}
			if waitFor != "" {
				_ = popup.Locator(waitFor).First().WaitFor()
			}
			popupPageID = id
			t, _ := popup.Title()
			popupTitle = strings.TrimSpace(t)
			popupURL = strings.TrimSpace(popup.URL())
			return nil
		}

		if err := clickFn(); err != nil {
			return err
		}
		if waitFor != "" {
			_ = page.Locator(waitFor).First().WaitFor()
		}
		return nil
	})
	return
}

func (m *BrowserSessionManager) Fill(ctx context.Context, sessionID, selector, text, waitFor string, timeoutMs int) error {
	return m.withActivePage(ctx, sessionID, func(s *browserSession, page playwright.Page) error {
		_ = s
		selector = strings.TrimSpace(selector)
		if selector == "" {
			return fmt.Errorf("selector is required")
		}
		loc := page.Locator(selector).First()
		fillOpts := playwright.LocatorFillOptions{}
		if timeoutMs > 0 {
			fillOpts.Timeout = playwright.Float(float64(timeoutMs))
		}
		if err := loc.Fill(text, fillOpts); err != nil {
			return err
		}
		waitFor = strings.TrimSpace(waitFor)
		if waitFor != "" {
			_ = page.Locator(waitFor).First().WaitFor()
		}
		return nil
	})
}

func (m *BrowserSessionManager) Extract(ctx context.Context, sessionID, selector, attribute string) (json.RawMessage, error) {
	var out json.RawMessage
	err := m.withActivePage(ctx, sessionID, func(s *browserSession, page playwright.Page) error {
		_ = s
		selector = strings.TrimSpace(selector)
		if selector == "" {
			return fmt.Errorf("selector is required")
		}
		attribute = strings.TrimSpace(attribute)
		if attribute == "" {
			attribute = "textContent"
		}

		expr := selectorAllExpr(attribute)
		result, err := page.EvalOnSelectorAll(selector, expr)
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

func (m *BrowserSessionManager) Dismiss(ctx context.Context, sessionID, kind, mode string, maxClicks int) (json.RawMessage, error) {
	if maxClicks <= 0 {
		maxClicks = 3
	}
	kind = strings.ToLower(strings.TrimSpace(kind))
	if kind == "" {
		kind = "cookies"
	}
	switch kind {
	case "cookies", "popups", "all":
	default:
		return nil, fmt.Errorf("kind must be cookies|popups|all")
	}
	mode = strings.ToLower(strings.TrimSpace(mode))
	if mode == "" {
		if kind == "popups" {
			mode = "close"
		} else {
			mode = "accept"
		}
	}
	switch mode {
	case "accept", "reject", "close":
	default:
		return nil, fmt.Errorf("mode must be accept|reject|close")
	}

	type clickedItem struct {
		Selector  string `json:"selector"`
		FrameURL  string `json:"frameUrl,omitempty"`
		FrameName string `json:"frameName,omitempty"`
	}
	clicked := make([]clickedItem, 0, maxClicks)

	err := m.withActivePage(ctx, sessionID, func(s *browserSession, page playwright.Page) error {
		_ = s
		selectors := dismissSelectors(kind, mode)
		if len(selectors) == 0 {
			return nil
		}

		frames := prioritizeFrames(page, kind)
		for _, fr := range frames {
			if len(clicked) >= maxClicks {
				break
			}
			for _, sel := range selectors {
				if len(clicked) >= maxClicks {
					break
				}
				if tryDismissClick(fr, sel, mode == "close") {
					clicked = append(clicked, clickedItem{
						Selector:  sel,
						FrameURL:  strings.TrimSpace(fr.URL()),
						FrameName: strings.TrimSpace(fr.Name()),
					})
					// Give the page a moment to apply DOM changes.
					page.WaitForTimeout(150)
				}
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	out := map[string]any{
		"clicked": clicked,
		"count":   len(clicked),
	}
	b, jerr := json.Marshal(out)
	if jerr != nil {
		return nil, jerr
	}
	return b, nil
}

func (m *BrowserSessionManager) Wait(ctx context.Context, sessionID, waitType, url, selector, state string, timeoutMs int, sleepMs int) error {
	_ = ctx
	waitType = strings.ToLower(strings.TrimSpace(waitType))
	url = strings.TrimSpace(url)
	selector = strings.TrimSpace(selector)
	state = strings.ToLower(strings.TrimSpace(state))

	if waitType == "" {
		switch {
		case selector != "":
			waitType = "selector"
		case url != "":
			waitType = "url"
		case sleepMs > 0:
			waitType = "timeout"
		case state != "":
			waitType = "load"
		default:
			waitType = "load"
		}
	}

	return m.withActivePage(ctx, sessionID, func(_ *browserSession, page playwright.Page) error {
		switch waitType {
		case "selector":
			if selector == "" {
				return fmt.Errorf("selector is required for waitType=selector")
			}
			opts := playwright.LocatorWaitForOptions{}
			if st := selectorState(state); st != nil {
				opts.State = st
			}
			if timeoutMs > 0 {
				opts.Timeout = playwright.Float(float64(timeoutMs))
			}
			return page.Locator(selector).First().WaitFor(opts)
		case "url":
			if url == "" {
				return fmt.Errorf("url is required for waitType=url")
			}
			opts := playwright.PageWaitForURLOptions{
				WaitUntil: playwright.WaitUntilStateDomcontentloaded,
			}
			if timeoutMs > 0 {
				opts.Timeout = playwright.Float(float64(timeoutMs))
			}
			return page.WaitForURL(url, opts)
		case "networkidle":
			opts := playwright.PageWaitForLoadStateOptions{
				State: playwright.LoadStateNetworkidle,
			}
			if timeoutMs > 0 {
				opts.Timeout = playwright.Float(float64(timeoutMs))
			}
			return page.WaitForLoadState(opts)
		case "load", "domcontentloaded":
			opts := playwright.PageWaitForLoadStateOptions{
				State: loadState(state),
			}
			if opts.State == nil {
				// Default to domcontentloaded: more reliable on modern sites.
				opts.State = playwright.LoadStateDomcontentloaded
			}
			if timeoutMs > 0 {
				opts.Timeout = playwright.Float(float64(timeoutMs))
			}
			return page.WaitForLoadState(opts)
		case "timeout", "sleep":
			if sleepMs <= 0 {
				if timeoutMs > 0 {
					sleepMs = timeoutMs
				} else {
					return fmt.Errorf("sleepMs is required for waitType=timeout")
				}
			}
			time.Sleep(time.Duration(sleepMs) * time.Millisecond)
			return nil
		default:
			return fmt.Errorf("unknown waitType: %s", waitType)
		}
	})
}

func (m *BrowserSessionManager) Hover(ctx context.Context, sessionID, selector string, timeoutMs int) error {
	return m.withActivePage(ctx, sessionID, func(_ *browserSession, page playwright.Page) error {
		selector = strings.TrimSpace(selector)
		if selector == "" {
			return fmt.Errorf("selector is required")
		}
		opts := playwright.LocatorHoverOptions{}
		if timeoutMs > 0 {
			opts.Timeout = playwright.Float(float64(timeoutMs))
		}
		return page.Locator(selector).First().Hover(opts)
	})
}

func (m *BrowserSessionManager) Press(ctx context.Context, sessionID, selector, key string, timeoutMs int) error {
	return m.withActivePage(ctx, sessionID, func(_ *browserSession, page playwright.Page) error {
		key = strings.TrimSpace(key)
		if key == "" {
			return fmt.Errorf("key is required")
		}
		selector = strings.TrimSpace(selector)
		if selector == "" {
			// Page-level key press.
			return page.Keyboard().Press(key)
		}
		opts := playwright.LocatorPressOptions{}
		if timeoutMs > 0 {
			opts.Timeout = playwright.Float(float64(timeoutMs))
		}
		return page.Locator(selector).First().Press(key, opts)
	})
}

func (m *BrowserSessionManager) Scroll(ctx context.Context, sessionID string, dx, dy int) error {
	return m.withActivePage(ctx, sessionID, func(_ *browserSession, page playwright.Page) error {
		return page.Mouse().Wheel(float64(dx), float64(dy))
	})
}

func (m *BrowserSessionManager) Select(ctx context.Context, sessionID, selector string, values []string, timeoutMs int) (json.RawMessage, error) {
	var out json.RawMessage
	err := m.withActivePage(ctx, sessionID, func(_ *browserSession, page playwright.Page) error {
		selector = strings.TrimSpace(selector)
		if selector == "" {
			return fmt.Errorf("selector is required")
		}
		clean := make([]string, 0, len(values))
		for _, v := range values {
			v = strings.TrimSpace(v)
			if v != "" {
				clean = append(clean, v)
			}
		}
		if len(clean) == 0 {
			return fmt.Errorf("value(s) are required")
		}
		opts := playwright.LocatorSelectOptionOptions{}
		if timeoutMs > 0 {
			opts.Timeout = playwright.Float(float64(timeoutMs))
		}
		vs := clean
		selected, err := page.Locator(selector).First().SelectOption(playwright.SelectOptionValues{
			ValuesOrLabels: &vs,
		}, opts)
		if err != nil {
			return err
		}
		b, err := json.Marshal(selected)
		if err != nil {
			return err
		}
		out = b
		return nil
	})
	return out, err
}

func (m *BrowserSessionManager) SetChecked(ctx context.Context, sessionID, selector string, checked bool, timeoutMs int) error {
	return m.withActivePage(ctx, sessionID, func(_ *browserSession, page playwright.Page) error {
		selector = strings.TrimSpace(selector)
		if selector == "" {
			return fmt.Errorf("selector is required")
		}
		if checked {
			opts := playwright.LocatorCheckOptions{}
			if timeoutMs > 0 {
				opts.Timeout = playwright.Float(float64(timeoutMs))
			}
			return page.Locator(selector).First().Check(opts)
		}
		opts := playwright.LocatorUncheckOptions{}
		if timeoutMs > 0 {
			opts.Timeout = playwright.Float(float64(timeoutMs))
		}
		return page.Locator(selector).First().Uncheck(opts)
	})
}

func (m *BrowserSessionManager) Upload(ctx context.Context, sessionID, selector, absPath string, timeoutMs int) error {
	return m.withActivePage(ctx, sessionID, func(_ *browserSession, page playwright.Page) error {
		selector = strings.TrimSpace(selector)
		if selector == "" {
			return fmt.Errorf("selector is required")
		}
		absPath = strings.TrimSpace(absPath)
		if absPath == "" || !filepath.IsAbs(absPath) {
			return fmt.Errorf("absPath must be an absolute path")
		}
		if _, err := os.Stat(absPath); err != nil {
			return err
		}
		opts := playwright.LocatorSetInputFilesOptions{}
		if timeoutMs > 0 {
			opts.Timeout = playwright.Float(float64(timeoutMs))
		}
		return page.Locator(selector).First().SetInputFiles(absPath, opts)
	})
}

func (m *BrowserSessionManager) Download(ctx context.Context, sessionID, selector, absPath string, timeoutMs int) (json.RawMessage, error) {
	var out json.RawMessage
	err := m.withActivePage(ctx, sessionID, func(_ *browserSession, page playwright.Page) error {
		selector = strings.TrimSpace(selector)
		if selector == "" {
			return fmt.Errorf("selector is required")
		}
		absPath = strings.TrimSpace(absPath)
		if absPath == "" || !filepath.IsAbs(absPath) {
			return fmt.Errorf("absPath must be an absolute path")
		}
		if err := os.MkdirAll(filepath.Dir(absPath), 0755); err != nil {
			return err
		}
		clickFn := func() error {
			opts := playwright.LocatorClickOptions{
				NoWaitAfter: playwright.Bool(true),
			}
			if timeoutMs > 0 {
				opts.Timeout = playwright.Float(float64(timeoutMs))
			}
			return page.Locator(selector).First().Click(opts)
		}
		edOpts := playwright.PageExpectDownloadOptions{}
		if timeoutMs > 0 {
			edOpts.Timeout = playwright.Float(float64(timeoutMs))
		}
		dl, err := page.ExpectDownload(clickFn, edOpts)
		if err != nil {
			return err
		}
		if err := dl.SaveAs(absPath); err != nil {
			return err
		}
		b, err := json.Marshal(map[string]string{
			"suggestedFilename": strings.TrimSpace(dl.SuggestedFilename()),
			"url":               strings.TrimSpace(dl.URL()),
		})
		if err != nil {
			return err
		}
		out = b
		return nil
	})
	return out, err
}

func (m *BrowserSessionManager) GoBack(ctx context.Context, sessionID string, timeoutMs int) (title string, finalURL string, err error) {
	err = m.withActivePage(ctx, sessionID, func(_ *browserSession, page playwright.Page) error {
		opts := playwright.PageGoBackOptions{}
		if timeoutMs > 0 {
			opts.Timeout = playwright.Float(float64(timeoutMs))
		}
		_, err := page.GoBack(opts)
		if err != nil {
			return err
		}
		t, _ := page.Title()
		title = strings.TrimSpace(t)
		finalURL = strings.TrimSpace(page.URL())
		return nil
	})
	return
}

func (m *BrowserSessionManager) GoForward(ctx context.Context, sessionID string, timeoutMs int) (title string, finalURL string, err error) {
	err = m.withActivePage(ctx, sessionID, func(_ *browserSession, page playwright.Page) error {
		opts := playwright.PageGoForwardOptions{}
		if timeoutMs > 0 {
			opts.Timeout = playwright.Float(float64(timeoutMs))
		}
		_, err := page.GoForward(opts)
		if err != nil {
			return err
		}
		t, _ := page.Title()
		title = strings.TrimSpace(t)
		finalURL = strings.TrimSpace(page.URL())
		return nil
	})
	return
}

func (m *BrowserSessionManager) Reload(ctx context.Context, sessionID string, timeoutMs int) (title string, finalURL string, err error) {
	err = m.withActivePage(ctx, sessionID, func(_ *browserSession, page playwright.Page) error {
		opts := playwright.PageReloadOptions{
			WaitUntil: playwright.WaitUntilStateDomcontentloaded,
		}
		if timeoutMs > 0 {
			opts.Timeout = playwright.Float(float64(timeoutMs))
		}
		_, err := page.Reload(opts)
		if err != nil {
			return err
		}
		t, _ := page.Title()
		title = strings.TrimSpace(t)
		finalURL = strings.TrimSpace(page.URL())
		return nil
	})
	return
}

func (m *BrowserSessionManager) TabNew(ctx context.Context, sessionID, url string, setActive bool, timeoutMs int) (pageID, title, finalURL string, err error) {
	err = m.withSession(ctx, sessionID, func(s *browserSession) error {
		if s.context == nil {
			return fmt.Errorf("browser context is nil")
		}
		p, err := s.context.NewPage()
		if err != nil {
			return err
		}
		configurePageDefaults(p)
		if s.viewportW > 0 && s.viewportH > 0 {
			_ = p.SetViewportSize(s.viewportW, s.viewportH)
		}
		id := uuid.NewString()[:8]
		if s.pages == nil {
			s.pages = make(map[string]playwright.Page)
		}
		s.pages[id] = p
		if setActive {
			s.activeID = id
		}
		url = strings.TrimSpace(url)
		if url != "" {
			gotoOpts := playwright.PageGotoOptions{WaitUntil: playwright.WaitUntilStateDomcontentloaded}
			if timeoutMs > 0 {
				gotoOpts.Timeout = playwright.Float(float64(timeoutMs))
			}
			if _, err := p.Goto(url, gotoOpts); err != nil {
				return err
			}
		}
		t, _ := p.Title()
		title = strings.TrimSpace(t)
		finalURL = strings.TrimSpace(p.URL())
		pageID = id
		return nil
	})
	return
}

func (m *BrowserSessionManager) TabList(ctx context.Context, sessionID string) (json.RawMessage, error) {
	var out json.RawMessage
	err := m.withSession(ctx, sessionID, func(s *browserSession) error {
		type tab struct {
			PageID string `json:"pageId"`
			URL    string `json:"url"`
			Title  string `json:"title"`
			Active bool   `json:"active"`
		}
		tabs := make([]tab, 0, len(s.pages))
		for id, p := range s.pages {
			if p == nil {
				continue
			}
			t, _ := p.Title()
			tabs = append(tabs, tab{
				PageID: id,
				URL:    strings.TrimSpace(p.URL()),
				Title:  strings.TrimSpace(t),
				Active: id == s.activeID,
			})
		}
		b, err := json.Marshal(tabs)
		if err != nil {
			return err
		}
		out = b
		return nil
	})
	return out, err
}

func (m *BrowserSessionManager) TabSwitch(ctx context.Context, sessionID, pageID string) error {
	return m.withSession(ctx, sessionID, func(s *browserSession) error {
		pageID = strings.TrimSpace(pageID)
		if pageID == "" {
			return fmt.Errorf("pageId is required")
		}
		if s.pages == nil || s.pages[pageID] == nil {
			return fmt.Errorf("tab not found: %s", pageID)
		}
		s.activeID = pageID
		return nil
	})
}

func (m *BrowserSessionManager) TabClose(ctx context.Context, sessionID, pageID string) error {
	return m.withSession(ctx, sessionID, func(s *browserSession) error {
		pageID = strings.TrimSpace(pageID)
		if pageID == "" {
			pageID = s.activeID
		}
		p := s.pages[pageID]
		if p == nil {
			return fmt.Errorf("tab not found: %s", pageID)
		}
		_ = p.Close()
		delete(s.pages, pageID)
		if s.activeID == pageID {
			s.activeID = ""
			for id := range s.pages {
				s.activeID = id
				break
			}
		}
		return nil
	})
}

func (m *BrowserSessionManager) StorageSave(ctx context.Context, sessionID, absPath string) error {
	return m.withSession(ctx, sessionID, func(s *browserSession) error {
		_ = ctx
		if s.context == nil {
			return fmt.Errorf("browser context is nil")
		}
		absPath = strings.TrimSpace(absPath)
		if absPath == "" || !filepath.IsAbs(absPath) {
			return fmt.Errorf("absPath must be an absolute path")
		}
		if err := os.MkdirAll(filepath.Dir(absPath), 0755); err != nil {
			return err
		}
		_, err := s.context.StorageState(absPath)
		return err
	})
}

func (m *BrowserSessionManager) StorageLoad(ctx context.Context, sessionID, absPath string) error {
	return m.withSession(ctx, sessionID, func(s *browserSession) error {
		_ = ctx
		if s.browser == nil {
			return fmt.Errorf("browser is nil")
		}
		absPath = strings.TrimSpace(absPath)
		if absPath == "" || !filepath.IsAbs(absPath) {
			return fmt.Errorf("absPath must be an absolute path")
		}
		if _, err := os.Stat(absPath); err != nil {
			return err
		}

		// Close existing pages/context and recreate a new context populated with storage state.
		for id, p := range s.pages {
			if p != nil {
				_ = p.Close()
			}
			delete(s.pages, id)
		}
		if s.context != nil {
			_ = s.context.Close()
		}

		var viewport *playwright.Size
		if s.viewportW > 0 && s.viewportH > 0 {
			viewport = &playwright.Size{Width: s.viewportW, Height: s.viewportH}
		}
		opts := playwright.BrowserNewContextOptions{
			AcceptDownloads:  playwright.Bool(true),
			Viewport:         viewport,
			ExtraHttpHeaders: s.extraHeaders,
			StorageStatePath: playwright.String(absPath),
		}
		if strings.TrimSpace(s.userAgent) != "" {
			opts.UserAgent = playwright.String(strings.TrimSpace(s.userAgent))
		}
		ctxx, err := s.browser.NewContext(opts)
		if err != nil {
			return err
		}
		ctxx.SetDefaultTimeout(defaultBrowserTimeoutMs)
		ctxx.SetDefaultNavigationTimeout(defaultBrowserTimeoutMs)
		s.context = ctxx

		p, err := ctxx.NewPage()
		if err != nil {
			_ = ctxx.Close()
			return err
		}
		configurePageDefaults(p)
		if s.viewportW > 0 && s.viewportH > 0 {
			_ = p.SetViewportSize(s.viewportW, s.viewportH)
		}
		id := uuid.NewString()[:8]
		if s.pages == nil {
			s.pages = make(map[string]playwright.Page)
		}
		s.pages[id] = p
		s.activeID = id
		return nil
	})
}

func (m *BrowserSessionManager) SetExtraHeaders(ctx context.Context, sessionID string, headers map[string]string) error {
	return m.withSession(ctx, sessionID, func(s *browserSession) error {
		if s.context == nil {
			return fmt.Errorf("browser context is nil")
		}
		if headers == nil {
			headers = map[string]string{}
		}
		s.extraHeaders = make(map[string]string, len(headers))
		for k, v := range headers {
			k = strings.TrimSpace(k)
			if k == "" {
				continue
			}
			s.extraHeaders[k] = v
		}
		return s.context.SetExtraHTTPHeaders(s.extraHeaders)
	})
}

func (m *BrowserSessionManager) SetViewport(ctx context.Context, sessionID string, width, height int) error {
	return m.withSession(ctx, sessionID, func(s *browserSession) error {
		if width <= 0 || height <= 0 {
			return fmt.Errorf("width and height must be > 0")
		}
		s.viewportW = width
		s.viewportH = height
		for _, p := range s.pages {
			if p != nil {
				_ = p.SetViewportSize(width, height)
			}
		}
		return nil
	})
}

func (m *BrowserSessionManager) ExtractLinks(ctx context.Context, sessionID, selector string) (json.RawMessage, error) {
	var out json.RawMessage
	err := m.withActivePage(ctx, sessionID, func(_ *browserSession, page playwright.Page) error {
		selector = strings.TrimSpace(selector)
		if selector == "" {
			selector = "a"
		}
		expr := `(elements) => elements.map(el => ({ text: ((el.innerText || el.textContent || "") + "").trim(), href: (el.href || el.getAttribute("href") || "") + "" }))`
		result, err := page.EvalOnSelectorAll(selector, expr)
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
	return m.withActivePage(ctx, sessionID, func(s *browserSession, page playwright.Page) error {
		_ = s
		if strings.TrimSpace(absPath) == "" || !filepath.IsAbs(absPath) {
			return fmt.Errorf("absPath must be an absolute path")
		}
		if err := os.MkdirAll(filepath.Dir(absPath), 0755); err != nil {
			return err
		}
		_, err := page.Screenshot(playwright.PageScreenshotOptions{
			Path:     playwright.String(absPath),
			FullPage: playwright.Bool(fullPage),
		})
		return err
	})
}

func (m *BrowserSessionManager) PDF(ctx context.Context, sessionID, absPath string) error {
	_ = ctx
	return m.withActivePage(ctx, sessionID, func(s *browserSession, page playwright.Page) error {
		_ = s
		if strings.TrimSpace(absPath) == "" || !filepath.IsAbs(absPath) {
			return fmt.Errorf("absPath must be an absolute path")
		}
		if err := os.MkdirAll(filepath.Dir(absPath), 0755); err != nil {
			return err
		}
		_, err := page.PDF(playwright.PagePdfOptions{
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

func (m *BrowserSessionManager) withActivePage(ctx context.Context, sessionID string, fn func(*browserSession, playwright.Page) error) error {
	return m.withSession(ctx, sessionID, func(s *browserSession) error {
		if s.pages == nil || strings.TrimSpace(s.activeID) == "" {
			return fmt.Errorf("no active tab in session")
		}
		page := s.pages[s.activeID]
		if page == nil {
			return fmt.Errorf("active tab not found: %s", s.activeID)
		}
		return fn(s, page)
	})
}

func configurePageDefaults(page playwright.Page) {
	if page == nil {
		return
	}
	page.SetDefaultTimeout(defaultBrowserTimeoutMs)
	page.SetDefaultNavigationTimeout(defaultBrowserTimeoutMs)
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
	pages := make([]playwright.Page, 0, len(s.pages))
	for _, p := range s.pages {
		if p != nil {
			pages = append(pages, p)
		}
	}
	ctxx := s.context
	browser := s.browser
	s.opMu.Unlock()

	// Best-effort close order.
	for _, p := range pages {
		_ = p.Close()
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

func selectorState(state string) *playwright.WaitForSelectorState {
	switch strings.ToLower(strings.TrimSpace(state)) {
	case "":
		return nil
	case "attached":
		return playwright.WaitForSelectorStateAttached
	case "detached":
		return playwright.WaitForSelectorStateDetached
	case "visible":
		return playwright.WaitForSelectorStateVisible
	case "hidden":
		return playwright.WaitForSelectorStateHidden
	default:
		return nil
	}
}

func loadState(state string) *playwright.LoadState {
	switch strings.ToLower(strings.TrimSpace(state)) {
	case "":
		return nil
	case "load":
		return playwright.LoadStateLoad
	case "domcontentloaded", "domcontent", "dom":
		return playwright.LoadStateDomcontentloaded
	case "networkidle", "network":
		return playwright.LoadStateNetworkidle
	default:
		return nil
	}
}

func prioritizeFrames(page playwright.Page, kind string) []playwright.Frame {
	if page == nil {
		return nil
	}
	frames := page.Frames()
	if len(frames) <= 1 {
		return frames
	}
	main := frames[0]
	rest := frames[1:]

	// Heuristic: cookie consent often lives in a dedicated iframe.
	var priority []playwright.Frame
	priority = append(priority, main)
	for _, fr := range rest {
		u := strings.ToLower(strings.TrimSpace(fr.URL()))
		n := strings.ToLower(strings.TrimSpace(fr.Name()))
		if strings.Contains(u, "consent") || strings.Contains(u, "cookie") || strings.Contains(u, "gdpr") ||
			strings.Contains(n, "consent") || strings.Contains(n, "cookie") || strings.Contains(n, "gdpr") {
			priority = append(priority, fr)
		}
	}
	for _, fr := range rest {
		u := strings.ToLower(strings.TrimSpace(fr.URL()))
		n := strings.ToLower(strings.TrimSpace(fr.Name()))
		if strings.Contains(u, "consent") || strings.Contains(u, "cookie") || strings.Contains(u, "gdpr") ||
			strings.Contains(n, "consent") || strings.Contains(n, "cookie") || strings.Contains(n, "gdpr") {
			continue
		}
		priority = append(priority, fr)
	}

	// When just trying to close generic popups, checking every frame can be slow/noisy.
	if kind == "popups" && len(priority) > 5 {
		return priority[:5]
	}
	return priority
}

func tryDismissClick(frame playwright.Frame, selector string, force bool) bool {
	if frame == nil || strings.TrimSpace(selector) == "" {
		return false
	}
	loc := frame.Locator(selector).First()
	if n, err := loc.Count(); err != nil || n == 0 {
		return false
	}
	timeout := 1200.0
	noWait := true
	if err := loc.Click(playwright.LocatorClickOptions{
		Timeout:     playwright.Float(timeout),
		NoWaitAfter: playwright.Bool(noWait),
		Force:       playwright.Bool(force),
	}); err != nil {
		return false
	}
	return true
}

func dismissSelectors(kind, mode string) []string {
	// Order matters: try well-known vendors first, then generic text-based matches.
	var out []string
	add := func(sel ...string) { out = append(out, sel...) }

	isCookies := kind == "cookies" || kind == "all"
	isPopups := kind == "popups" || kind == "all"

	if isCookies {
		switch mode {
		case "accept":
			add(
				// OneTrust
				`#onetrust-accept-btn-handler`,
				`#onetrust-accept-btn-handler button`,
				`#onetrust-banner-sdk button#onetrust-accept-btn-handler`,
				// Cookiebot
				`#CybotCookiebotDialogBodyLevelButtonAccept`,
				`#CybotCookiebotDialogBodyButtonAccept`,
				`#CybotCookiebotDialogBodyLevelButtonLevelOptinAllowAll`,
				// Didomi
				`#didomi-notice-agree-button`,
				`button#didomi-notice-agree-button`,
				// Quantcast / IAB
				`button:has-text("I agree")`,
				`button:has-text("Accept all")`,
				`button:has-text("Accept All")`,
				`button:has-text("Accept")`,
				`button:has-text("Agree")`,
				`button:has-text("OK")`,
				`button:has-text("Got it")`,
				`button:has-text("Continue")`,
				`text=Accept all`,
				`text=I agree`,
			)
		case "reject":
			add(
				// OneTrust
				`#onetrust-reject-all-handler`,
				`#onetrust-reject-all-handler button`,
				`#onetrust-banner-sdk button#onetrust-reject-all-handler`,
				// Cookiebot
				`#CybotCookiebotDialogBodyLevelButtonReject`,
				`#CybotCookiebotDialogBodyButtonDecline`,
				// Didomi
				`#didomi-notice-disagree-button`,
				`button#didomi-notice-disagree-button`,
				// Generic
				`button:has-text("Reject all")`,
				`button:has-text("Reject")`,
				`button:has-text("Decline")`,
				`button:has-text("Deny")`,
				`button:has-text("Only necessary")`,
				`button:has-text("Necessary only")`,
				`text=Reject all`,
			)
		case "close":
			// Some cookie dialogs are best dismissed via close button.
			add(
				`button[aria-label*="close" i]`,
				`[aria-label*="close" i]`,
				`button:has-text("Close")`,
				`text=Close`,
				`button:has-text("×")`,
			)
		}
	}

	if isPopups {
		switch mode {
		case "close", "accept", "reject":
			add(
				`button[aria-label*="close" i]`,
				`[aria-label*="close" i]`,
				`button:has-text("Close")`,
				`button:has-text("No thanks")`,
				`button:has-text("Not now")`,
				`button:has-text("Dismiss")`,
				`button:has-text("×")`,
				`[data-testid*="close" i]`,
				`[data-test*="close" i]`,
			)
		}
	}

	return out
}
