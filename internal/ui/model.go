// Package ui implements the Bubble Tea models for the GitHub TUI.
package ui

import (
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/huy-tran/github-tui/internal/gh"
)

type screen int

const (
	screenRepos screen = iota
	screenDetail
	screenRunDetail
	screenPRDetail
	screenMyPRs
	screenNotifs
	screenIssueDetail
)

// Model is the root model dispatching between screens and owning the chrome
// (title bar, status footer, help overlay, loading spinner).
type Model struct {
	screen screen
	login  string
	theme  Theme
	dryRun bool

	repos       reposModel
	detail      detailModel
	runDetail   runDetailModel
	prDetail    prDetailModel
	issueDetail issueDetailModel
	myPRs       myPRsModel
	notifs      notifsModel
	palette     paletteModel
	spinner     spinner.Model

	// prReturn is the screen to return to when leaving the PR detail (it can be
	// opened from the repo PRs tab or the My PRs dashboard).
	prReturn screen

	showHelp bool

	width  int
	height int

	lastErr error
}

// New constructs the root model for the given theme.
func New(theme Theme) Model {
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = accentStyle
	return Model{
		screen:   screenRepos,
		theme:    theme,
		repos:    newReposModel(theme),
		myPRs:    newMyPRsModel(theme),
		notifs:   newNotifsModel(theme),
		palette:  newPaletteModel(theme),
		prReturn: screenDetail,
		spinner:  sp,
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(
		loadUserCmd(),
		loadReposCacheCmd(), // instant: show cached repos while the network loads
		loadReposCmd(),
		loadVulnsCacheCmd(), // instant: show cached vulnerability counts ('v' re-scans)
		m.spinner.Tick,
		autoRefreshTickCmd(),
	)
}

// bodyHeight is the space between the title bar and the status footer. Four
// chrome rows are reserved: the title bar, the footer, and a blank spacer row
// above and below the body for breathing room.
func (m Model) bodyHeight() int {
	if m.height-4 < 1 {
		return 1
	}
	return m.height - 4
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.repos.setSize(msg.Width, m.bodyHeight())
		m.myPRs.setSize(msg.Width, m.bodyHeight())
		m.notifs.setSize(msg.Width, m.bodyHeight())
		switch m.screen {
		case screenDetail:
			m.detail.setSize(msg.Width, m.bodyHeight())
		case screenRunDetail:
			m.detail.setSize(msg.Width, m.bodyHeight())
			m.runDetail.setSize(msg.Width, m.bodyHeight())
		case screenPRDetail:
			m.detail.setSize(msg.Width, m.bodyHeight())
			m.prDetail.setSize(msg.Width, m.bodyHeight())
		case screenIssueDetail:
			m.detail.setSize(msg.Width, m.bodyHeight())
			m.issueDetail.setSize(msg.Width, m.bodyHeight())
		}
		return m, nil

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		if m.loadingActive() {
			return m, cmd
		}
		return m, nil // stop the animation cleanly

	case autoRefreshTickMsg:
		// Silently re-poll the active screen when it has running work, then
		// reschedule. The reload commands don't set loading flags, so data
		// updates in place (cursor/sort/scroll preserved) with no spinner.
		cmds := append(m.autoRefreshCmds(), autoRefreshTickCmd())
		return m, tea.Batch(cmds...)

	case tea.KeyMsg:
		return m.handleKey(msg)

	case userLoadedMsg:
		m.login = msg.login
		m.repos.setAccount(msg.login)
		m.detail.login = msg.login
		m.detail.account = msg.login
		m.myPRs.account = msg.login
		m.notifs.account = msg.login
		return m, nil

	case reposLoadedMsg:
		m.repos.setRepos(msg.repos)
		return m, nil

	case reposCacheLoadedMsg:
		m.repos.setReposFromCache(msg.repos, msg.savedAt)
		return m, nil

	case vulnsLoadedMsg:
		m.repos.setVulnCounts(msg.counts)
		return m, nil

	case vulnsCacheLoadedMsg:
		m.repos.setVulnCountsFromCache(msg.counts, msg.savedAt)
		return m, nil

	case myPRsLoadedMsg:
		m.myPRs.setData(msg.review, msg.authored)
		return m, nil

	case notifsLoadedMsg:
		m.notifs.setData(msg.notifs, msg.page)
		return m, nil

	case notifActionDoneMsg:
		if msg.err != nil {
			m.notifs.message = "mark read failed: " + firstLine(msg.err.Error())
		}
		return m, nil

	case notifsMarkedAllMsg:
		m.notifs.markedAll(msg.err)
		return m, nil

	case prsLoadedMsg:
		if msg.repo == m.detail.repo.NameWithOwner {
			m.detail.setPRs(msg.prs)
		}
		return m, nil

	case runsLoadedMsg:
		if msg.repo == m.detail.repo.NameWithOwner {
			m.detail.setRuns(msg.runs)
		}
		return m, nil

	case issuesLoadedMsg:
		if msg.repo == m.detail.repo.NameWithOwner {
			m.detail.setIssues(msg.issues)
		}
		return m, nil

	case securityLoadedMsg:
		if msg.repo == m.detail.repo.NameWithOwner {
			m.detail.setSecurity(msg.alerts, msg.unavailable)
		}
		return m, nil

	case dispatchInfoLoadedMsg:
		if m.screen == screenDetail && msg.repo == m.detail.repo.NameWithOwner {
			m.detail.dispatch.setInfo(msg.workflows, msg.defaultBranch, msg.err)
		}
		return m, nil

	case dispatchDoneMsg:
		if m.screen == screenDetail && msg.repo == m.detail.repo.NameWithOwner {
			return m, m.detail.dispatchDone(msg.name, msg.err)
		}
		return m, nil

	case openIssueMsg:
		m.issueDetail = newIssueDetailModel(msg.repo, msg.issue, m.login, m.theme)
		m.issueDetail.setSize(m.width, m.bodyHeight())
		m.screen = screenIssueDetail
		return m, tea.Batch(m.issueDetail.initCmd(), m.spinner.Tick)

	case issueDetailLoadedMsg:
		if m.screen == screenIssueDetail && msg.number == m.issueDetail.issue.Number {
			m.issueDetail.setDetail(msg.detail)
		}
		return m, nil

	case issueActionDoneMsg:
		if m.screen == screenIssueDetail && msg.number == m.issueDetail.issue.Number {
			return m, m.issueDetail.actionDone(msg.action, msg.err)
		}
		return m, nil

	case openRunMsg:
		m.runDetail = newRunDetailModel(msg.repo, msg.run, m.login, m.theme)
		m.runDetail.setSize(m.width, m.bodyHeight())
		m.screen = screenRunDetail
		return m, tea.Batch(m.runDetail.initCmd(), m.spinner.Tick)

	case runDetailLoadedMsg:
		if m.screen == screenRunDetail && msg.runID == m.runDetail.run.DatabaseID {
			m.runDetail.setDetail(msg.detail)
		}
		return m, nil

	case runLogLoadedMsg:
		if m.screen == screenRunDetail && msg.runID == m.runDetail.run.DatabaseID {
			m.runDetail.setLog(msg.jobID, msg.log)
		}
		return m, nil

	case runActionDoneMsg:
		if m.screen == screenRunDetail && msg.runID == m.runDetail.run.DatabaseID {
			return m, m.runDetail.actionDone(msg.action, msg.err)
		}
		return m, nil

	case openPRMsg:
		m.prReturn = m.screen // remember whether we came from a repo or My PRs
		m.prDetail = newPRDetailModel(msg.repo, msg.pr, m.login, m.theme)
		m.prDetail.setSize(m.width, m.bodyHeight())
		m.screen = screenPRDetail
		return m, tea.Batch(m.prDetail.initCmd(), m.spinner.Tick)

	case prDetailLoadedMsg:
		if m.screen == screenPRDetail && msg.number == m.prDetail.pr.Number {
			m.prDetail.setDetail(msg.detail)
		}
		return m, nil

	case prDiffLoadedMsg:
		if m.screen == screenPRDetail && msg.number == m.prDetail.pr.Number {
			m.prDetail.setDiff(msg.diff)
		}
		return m, nil

	case prActionDoneMsg:
		if m.screen == screenPRDetail && msg.number == m.prDetail.pr.Number {
			return m, m.prDetail.actionDone(msg.action, msg.err)
		}
		return m, nil

	case errMsg:
		m.applyErr(msg)
		return m, nil
	}

	return m.forward(msg)
}

func (m *Model) applyErr(msg errMsg) {
	m.lastErr = msg
	switch msg.context {
	case "loading repositories":
		m.repos.loading = false
		m.repos.refreshing = false
	case "loading my PRs":
		m.myPRs.loading = false
		m.myPRs.err = msg.err
	case "loading notifications":
		m.notifs.loading = false
		m.notifs.err = msg.err
	case "loading pull requests":
		m.detail.loadingPRs = false
		m.detail.prErr = msg.err
	case "loading workflow runs":
		m.detail.loadingRuns = false
		m.detail.runErr = msg.err
	case "loading issues":
		m.detail.loadingIssues = false
		m.detail.issueErr = msg.err
	case "loading vulnerabilities":
		m.detail.loadingSec = false
		m.detail.secErr = msg.err
	case "loading issue":
		m.issueDetail.loading = false
		m.issueDetail.err = msg.err
	case "loading workflow run":
		m.runDetail.loading = false
		m.runDetail.err = msg.err
	case "loading logs":
		m.runDetail.logsLoading = false
		m.runDetail.logsErr = msg.err
	case "loading pull request":
		m.prDetail.loading = false
		m.prDetail.err = msg.err
	case "loading diff":
		m.prDetail.diffLoading = false
		m.prDetail.diffErr = msg.err
	}
}

// loadingActive reports whether the spinner should keep ticking.
func (m Model) loadingActive() bool {
	switch m.screen {
	case screenRepos:
		return m.repos.loading
	case screenRunDetail:
		return m.runDetail.loading
	case screenPRDetail:
		return m.prDetail.loading
	case screenMyPRs:
		return m.myPRs.loading
	case screenNotifs:
		return m.notifs.loading
	case screenIssueDetail:
		return m.issueDetail.loading
	default:
		return m.detail.loadingPRs || m.detail.loadingRuns || m.detail.loadingIssues || m.detail.loadingSec
	}
}

// sortingActive reports whether the active table's sort ribbon is up.
func (m *Model) sortingActive() bool {
	switch m.screen {
	case screenRepos:
		return m.repos.table.Sorting()
	case screenRunDetail:
		return m.runDetail.table.Sorting()
	case screenPRDetail:
		return false // no table on this screen
	case screenMyPRs:
		return m.myPRs.table.Sorting()
	case screenNotifs:
		return m.notifs.table.Sorting()
	case screenIssueDetail:
		return false // no table on this screen
	default:
		return m.detail.activeTable().Sorting()
	}
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.String() == "ctrl+c" {
		return m, tea.Quit
	}

	if m.showHelp {
		switch msg.String() {
		case "?", "esc", "q":
			m.showHelp = false
		}
		return m, nil
	}

	// Command palette: captures keys while open; ctrl+k opens it from anywhere.
	if m.palette.active {
		switch msg.String() {
		case "esc", "ctrl+k":
			m.palette.close()
			return m, nil
		case "enter":
			if it, ok := m.palette.selected(); ok {
				m.palette.close()
				return m.gotoPaletteTarget(it)
			}
			return m, nil
		case "up", "ctrl+p":
			m.palette.move(-1)
			return m, nil
		case "down", "ctrl+n":
			m.palette.move(1)
			return m, nil
		default:
			return m, m.palette.updateInput(msg)
		}
	}
	if msg.String() == "ctrl+k" {
		m.palette.open(m.repos.repos)
		return m, textinput.Blink
	}

	// `?` opens help, except where the key belongs to a text field / sort ribbon.
	if msg.String() == "?" && m.canOpenHelp() {
		m.showHelp = true
		return m, nil
	}

	// While a text composer / prompt is open, let every key reach it (don't let
	// the global ctrl+o/ctrl+f shortcuts steal keystrokes mid-compose).
	if m.screen == screenPRDetail && m.prDetail.capturing() {
		return m, m.prDetail.Update(msg)
	}
	if m.screen == screenIssueDetail && m.issueDetail.capturing() {
		return m, m.issueDetail.Update(msg)
	}

	// ctrl+o opens the current selection in the browser, on any screen.
	if msg.String() == "ctrl+o" {
		if url := m.currentBrowserURL(); url != "" {
			return m, openURLCmd(url)
		}
		return m, nil
	}

	// ctrl+f re-fetches the current screen's data.
	if msg.String() == "ctrl+f" {
		return m, m.startRefresh()
	}

	switch m.screen {
	case screenRepos:
		if !m.repos.table.Filtering() && !m.repos.table.Sorting() {
			switch msg.String() {
			case "q":
				return m, tea.Quit
			case "p":
				return m.openMyPRs()
			case "n":
				return m.openNotifs()
			case "v":
				if !m.repos.vulnsScanning && len(m.repos.repos) > 0 {
					m.repos.beginVulnScan()
					return m, loadVulnsCmd(m.repos.repos)
				}
				return m, nil
			case "enter":
				if repo, ok := m.repos.selected(); ok {
					m.detail = newDetailModel(repo, m.login, m.login, m.theme)
					m.detail.setSize(m.width, m.bodyHeight())
					m.screen = screenDetail
					return m, tea.Batch(m.detail.initCmd(), m.spinner.Tick)
				}
			}
		}
		return m, m.repos.Update(msg)

	case screenDetail:
		if !m.detail.busy() {
			switch msg.String() {
			case "q":
				return m, tea.Quit
			case "esc", "backspace":
				m.screen = screenRepos
				return m, nil
			}
		}
		return m, m.detail.Update(msg)

	case screenRunDetail:
		if !m.runDetail.busy() {
			switch msg.String() {
			case "q":
				return m, tea.Quit
			case "esc", "backspace":
				// Pop one level: logs view → jobs table → workflows tab.
				if m.runDetail.inLogs() {
					m.runDetail.backToJobs()
					return m, nil
				}
				m.screen = screenDetail
				return m, nil
			}
		}
		return m, m.runDetail.Update(msg)

	case screenPRDetail:
		// While a prompt or the review composer is up, esc cancels it (handled
		// by the screen) and q/typed text must reach it; otherwise esc goes
		// back and q quits.
		if !m.prDetail.capturing() {
			switch msg.String() {
			case "q":
				return m, tea.Quit
			case "esc", "backspace":
				m.screen = m.prReturn // back to repo PRs tab or My PRs dashboard
				return m, nil
			}
		}
		return m, m.prDetail.Update(msg)

	case screenMyPRs:
		if !m.myPRs.table.Sorting() && !m.myPRs.table.Filtering() {
			switch msg.String() {
			case "q":
				return m, tea.Quit
			case "esc", "backspace":
				m.screen = screenRepos
				return m, nil
			case "t":
				m.myPRs.toggleAuthored()
				return m, nil
			case "enter":
				if repo, pr, ok := m.myPRs.selected(); ok {
					return m, func() tea.Msg { return openPRMsg{repo: repo, pr: pr} }
				}
				return m, nil
			}
		}
		return m, m.myPRs.Update(msg)

	case screenNotifs:
		// Mark-all confirmation prompt captures keys.
		if m.notifs.pendingMarkAll {
			switch msg.String() {
			case "y":
				m.notifs.pendingMarkAll = false
				return m, markAllNotifsReadCmd()
			case "n", "esc":
				m.notifs.pendingMarkAll = false
			}
			return m, nil
		}
		if !m.notifs.table.Sorting() && !m.notifs.table.Filtering() {
			switch msg.String() {
			case "q":
				return m, tea.Quit
			case "esc", "backspace":
				m.screen = screenRepos
				return m, nil
			case "A":
				if len(m.notifs.notifs) > 0 {
					m.notifs.pendingMarkAll = true
				}
				return m, nil
			case "f":
				m.notifs.cycleFilter()
				return m, nil
			case "m":
				if !m.notifs.loadingMore && !m.notifs.loading {
					m.notifs.loadingMore = true
					return m, loadNotifsPageCmd(m.notifs.page + 1)
				}
				return m, nil
			case "x":
				if n, ok := m.notifs.selected(); ok {
					m.notifs.removeByID(n.ID) // optimistic
					return m, markNotifReadCmd(n.ID)
				}
				return m, nil
			case "enter":
				// Drill into the PR detail for PR notifications; otherwise a
				// no-op (ctrl+o opens any item in the browser).
				if n, ok := m.notifs.selected(); ok {
					if num, isPR := n.PRNumber(); isPR {
						return m, func() tea.Msg {
							return openPRMsg{repo: n.RepoName(), pr: gh.PR{Number: num, Title: n.Subject.Title, URL: n.WebURL()}}
						}
					}
				}
				return m, nil
			}
		}
		return m, m.notifs.Update(msg)

	case screenIssueDetail:
		if !m.issueDetail.capturing() {
			switch msg.String() {
			case "q":
				return m, tea.Quit
			case "esc", "backspace":
				m.screen = screenDetail
				return m, nil
			}
		}
		return m, m.issueDetail.Update(msg)
	}
	return m, nil
}

// gotoPaletteTarget navigates to the chosen command-palette item.
func (m Model) gotoPaletteTarget(it paletteItem) (tea.Model, tea.Cmd) {
	switch it.kind {
	case paletteRepo:
		m.detail = newDetailModel(it.repo, m.login, m.login, m.theme)
		m.detail.setSize(m.width, m.bodyHeight())
		m.screen = screenDetail
		return m, tea.Batch(m.detail.initCmd(), m.spinner.Tick)
	default: // paletteScreen
		switch it.screen {
		case screenMyPRs:
			return m.openMyPRs()
		case screenNotifs:
			return m.openNotifs()
		default:
			m.screen = screenRepos
			return m, nil
		}
	}
}

// openMyPRs switches to the My PRs dashboard, loading it on first visit.
func (m Model) openMyPRs() (tea.Model, tea.Cmd) {
	m.screen = screenMyPRs
	m.myPRs.setSize(m.width, m.bodyHeight())
	if m.myPRs.loaded.IsZero() && !m.myPRs.loading {
		m.myPRs.loading = true
		return m, tea.Batch(loadMyPRsCmd(), m.spinner.Tick)
	}
	return m, nil
}

// openNotifs switches to the notifications inbox, loading it on first visit.
func (m Model) openNotifs() (tea.Model, tea.Cmd) {
	m.screen = screenNotifs
	m.notifs.setSize(m.width, m.bodyHeight())
	if m.notifs.loaded.IsZero() && !m.notifs.loading {
		m.notifs.loading = true
		return m, tea.Batch(loadNotifsCmd(), m.spinner.Tick)
	}
	return m, nil
}

// autoRefreshCmds returns silent reload commands for the active screen when it
// has running work (no loading flags set, so the refresh is invisible). It
// skips screens that are idle, loading, or mid-action.
func (m *Model) autoRefreshCmds() []tea.Cmd {
	switch m.screen {
	case screenDetail:
		if m.detail.active == tabWorkflows && !m.detail.loadingRuns && m.detail.hasActiveRun() {
			return []tea.Cmd{loadRunsCmd(m.detail.repo.NameWithOwner)}
		}
	case screenRunDetail:
		if !m.runDetail.loading && !m.runDetail.working && m.runDetail.isActive() {
			return []tea.Cmd{loadRunDetailCmd(m.runDetail.repo, m.runDetail.run.DatabaseID)}
		}
	}
	return nil
}

// startRefresh re-fetches the active screen's data, showing the spinner while
// the request is in flight. Filter and sort selections are preserved because
// the reload feeds the same view models, which re-apply them.
func (m *Model) startRefresh() tea.Cmd {
	switch m.screen {
	case screenRepos:
		m.repos.loading = true
		// Note: vuln counts are NOT refetched here - they stay cached until 'v'.
		return tea.Batch(loadReposCmd(), m.spinner.Tick)
	case screenDetail:
		repo := m.detail.repo.NameWithOwner
		m.detail.loadingPRs = true
		m.detail.loadingRuns = true
		m.detail.loadingIssues = true
		m.detail.loadingSec = true
		m.detail.prErr = nil
		m.detail.runErr = nil
		m.detail.issueErr = nil
		m.detail.secErr = nil
		m.detail.secDisabled = false
		return tea.Batch(loadPRsCmd(repo), loadRunsCmd(repo), loadIssuesCmd(repo), loadSecurityCmd(repo), m.spinner.Tick)
	case screenRunDetail:
		m.runDetail.loading = true
		m.runDetail.err = nil
		m.runDetail.logsLoaded = false // re-fetch logs lazily on next open
		return tea.Batch(m.runDetail.initCmd(), m.spinner.Tick)
	case screenPRDetail:
		m.prDetail.loading = true
		m.prDetail.err = nil
		m.prDetail.diffLoaded = false // re-fetch diff lazily on next toggle
		return tea.Batch(m.prDetail.initCmd(), m.spinner.Tick)
	case screenMyPRs:
		m.myPRs.loading = true
		m.myPRs.err = nil
		return tea.Batch(loadMyPRsCmd(), m.spinner.Tick)
	case screenNotifs:
		m.notifs.loading = true
		m.notifs.err = nil
		m.notifs.message = ""
		return tea.Batch(loadNotifsCmd(), m.spinner.Tick)
	case screenIssueDetail:
		m.issueDetail.loading = true
		m.issueDetail.err = nil
		return tea.Batch(m.issueDetail.initCmd(), m.spinner.Tick)
	}
	return nil
}

// currentBrowserURL resolves the URL the ctrl+o shortcut should open.
func (m *Model) currentBrowserURL() string {
	switch m.screen {
	case screenRepos:
		if r, ok := m.repos.selected(); ok {
			return "https://github.com/" + r.NameWithOwner
		}
	case screenDetail:
		return m.detail.activeTable().SelectedID()
	case screenRunDetail:
		return m.runDetail.browserURL()
	case screenPRDetail:
		return m.prDetail.browserURL()
	case screenMyPRs:
		return m.myPRs.table.SelectedID()
	case screenNotifs:
		if n, ok := m.notifs.selected(); ok {
			return n.WebURL()
		}
	case screenIssueDetail:
		return m.issueDetail.browserURL()
	}
	return ""
}

// filteringActive reports whether the active screen's table is mid-filter (so
// typed keys, incl. '?', must reach the filter input).
func (m *Model) filteringActive() bool {
	switch m.screen {
	case screenRepos:
		return m.repos.table.Filtering()
	case screenMyPRs:
		return m.myPRs.table.Filtering()
	case screenNotifs:
		return m.notifs.table.Filtering()
	case screenDetail:
		return m.detail.activeTable().Filtering()
	}
	return false
}

func (m *Model) canOpenHelp() bool {
	if m.sortingActive() {
		return false
	}
	if m.filteringActive() {
		return false
	}
	if m.screen == screenPRDetail && m.prDetail.capturing() {
		return false
	}
	if m.screen == screenRunDetail && m.runDetail.confirming() {
		return false
	}
	if m.screen == screenIssueDetail && m.issueDetail.capturing() {
		return false
	}
	if m.screen == screenNotifs && m.notifs.pendingMarkAll {
		return false
	}
	if m.screen == screenDetail && m.detail.dispatch.active {
		return false
	}
	return true
}

func (m Model) forward(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m.screen {
	case screenRepos:
		return m, m.repos.Update(msg)
	case screenDetail:
		return m, m.detail.Update(msg)
	}
	return m, nil
}

// --- view -----------------------------------------------------------------

func (m Model) View() string {
	if m.width == 0 || m.height == 0 {
		return "" // wait for the first WindowSizeMsg
	}
	title := titleBar(m.width, m.dryRun)
	footer := statusFooter(m.width, m.theme, m.snapshot())
	bodyH := m.bodyHeight()

	var body string
	switch {
	case m.palette.active:
		body = m.palette.View(m.width, bodyH)
	case m.showHelp:
		body = renderHelpOverlay(m.width, bodyH, m.helpSections())
	case m.fatalReposError():
		body = lipgloss.Place(m.width, bodyH, lipgloss.Center, lipgloss.Center,
			errorStyle.Render("Error: "+m.lastErr.Error())+"\n\n"+
				mutedStyleFor(m.theme).Render("Check `gh auth status`. Press q to quit."))
	default:
		if loading, thing := m.activeLoading(); loading {
			body = lipgloss.Place(m.width, bodyH, lipgloss.Center, lipgloss.Center,
				m.spinner.View()+" loading "+thing+"...")
		} else {
			body = m.activeView()
		}
	}

	body = fitHeight(body, bodyH)
	// Blank spacer rows above and below the body separate it from the title bar
	// and the status footer.
	return strings.Join([]string{title, "", body, "", footer}, "\n")
}

func (m Model) fatalReposError() bool {
	return m.screen == screenRepos && m.lastErr != nil && !m.repos.loading && len(m.repos.repos) == 0
}

func (m Model) activeLoading() (bool, string) {
	switch m.screen {
	case screenRepos:
		return m.repos.Loading()
	case screenRunDetail:
		return m.runDetail.Loading()
	case screenPRDetail:
		return m.prDetail.Loading()
	case screenMyPRs:
		return m.myPRs.Loading()
	case screenNotifs:
		return m.notifs.Loading()
	case screenIssueDetail:
		return m.issueDetail.Loading()
	default:
		return m.detail.Loading()
	}
}

func (m Model) activeView() string {
	switch m.screen {
	case screenRepos:
		return m.repos.View()
	case screenRunDetail:
		return m.runDetail.View()
	case screenPRDetail:
		return m.prDetail.View()
	case screenMyPRs:
		return m.myPRs.View()
	case screenNotifs:
		return m.notifs.View()
	case screenIssueDetail:
		return m.issueDetail.View()
	default:
		return m.detail.View()
	}
}

func (m Model) snapshot() Snapshot {
	switch m.screen {
	case screenRepos:
		return m.repos.snapshot()
	case screenRunDetail:
		return m.runDetail.snapshot()
	case screenPRDetail:
		return m.prDetail.snapshot()
	case screenMyPRs:
		return m.myPRs.snapshot()
	case screenNotifs:
		return m.notifs.snapshot()
	case screenIssueDetail:
		return m.issueDetail.snapshot()
	default:
		return m.detail.snapshot()
	}
}

func (m Model) helpSections() []helpSection {
	sections := []helpSection{globalHelp()}
	switch m.screen {
	case screenRepos:
		sections = append(sections, m.repos.helpSections()...)
	case screenRunDetail:
		sections = append(sections, m.runDetail.helpSections()...)
	case screenPRDetail:
		sections = append(sections, m.prDetail.helpSections()...)
	case screenMyPRs:
		sections = append(sections, m.myPRs.helpSections()...)
	case screenNotifs:
		sections = append(sections, m.notifs.helpSections()...)
	case screenIssueDetail:
		sections = append(sections, m.issueDetail.helpSections()...)
	default:
		sections = append(sections, m.detail.helpSections()...)
	}
	return sections
}

// fitHeight pads or crops s to exactly h lines so the footer stays anchored.
func fitHeight(s string, h int) string {
	lines := strings.Split(s, "\n")
	if len(lines) > h {
		lines = lines[:h]
	}
	for len(lines) < h {
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n")
}
