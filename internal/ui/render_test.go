package ui

import (
	"strconv"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/huy-tran/github-tui/internal/gh"
)

func step(t *testing.T, m Model, msg tea.Msg) Model {
	t.Helper()
	nm, _ := m.Update(msg)
	return nm.(Model)
}

func key(s string) tea.KeyMsg {
	switch s {
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	default:
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
	}
}

func sampleRepos() []gh.Repo {
	now := time.Now()
	return []gh.Repo{
		{Name: "hello-world", NameWithOwner: "octocat/hello-world", Description: "Laravel framework", PushedAt: now.Add(-2 * time.Hour)},
		{Name: "widget-api", NameWithOwner: "acme/widget-api", IsPrivate: true, PushedAt: now.Add(-30 * time.Minute)},
		{Name: "a-really-long-repository-name-that-should-truncate", NameWithOwner: "acme/a-really-long-repository-name-that-should-truncate", IsPrivate: true, PushedAt: now.Add(-72 * time.Hour)},
	}
}

func assertLayout(t *testing.T, out string, w, h int) {
	t.Helper()
	lines := strings.Split(out, "\n")
	if len(lines) != h {
		t.Errorf("expected %d rows, got %d", h, len(lines))
	}
	for i, ln := range lines {
		if vw := lineWidth(ln); vw > w {
			t.Errorf("line %d exceeds width %d: %d", i, w, vw)
		}
	}
}

func lineWidth(s string) int { return len([]rune(stripANSI(s))) }

func TestReposRender(t *testing.T) {
	const w, h = 90, 24
	m := New(darkTheme)
	m = step(t, m, tea.WindowSizeMsg{Width: w, Height: h})
	m = step(t, m, userLoadedMsg{login: "octocat"})
	m = step(t, m, reposLoadedMsg{repos: sampleRepos()})

	out := m.View()
	assertLayout(t, out, w, h)
	t.Log("\n" + stripANSI(out))
}

func TestDetailRender(t *testing.T) {
	const w, h = 90, 24
	now := time.Now()
	m := New(darkTheme)
	m = step(t, m, tea.WindowSizeMsg{Width: w, Height: h})
	m = step(t, m, userLoadedMsg{login: "octocat"})
	m = step(t, m, reposLoadedMsg{repos: sampleRepos()})
	m = step(t, m, key("enter")) // open first repo

	repo := m.detail.repo.NameWithOwner
	m = step(t, m, prsLoadedMsg{repo: repo, prs: []gh.PR{
		{Number: 42, Title: "Add a brand new feature to the system", HeadRefName: "feat/x", UpdatedAt: now.Add(-time.Hour), URL: "https://x/pr/42"},
	}})
	m = step(t, m, runsLoadedMsg{repo: repo, runs: []gh.Run{
		{DatabaseID: 1, WorkflowName: "CI", Status: "completed", Conclusion: "success", HeadBranch: "main", Event: "push", CreatedAt: now.Add(-20 * time.Minute), URL: "https://x/1"},
		{DatabaseID: 2, WorkflowName: "Deploy", Status: "in_progress", HeadBranch: "main", Event: "workflow_dispatch", CreatedAt: now.Add(-5 * time.Minute), URL: "https://x/2"},
	}})

	// PRs tab (toggle reviewer filter off so the sample PR shows).
	m = step(t, m, key("t"))
	out := m.View()
	assertLayout(t, out, w, h)
	t.Log("\nPRs tab:\n" + stripANSI(out))

	// Workflows tab.
	m = step(t, m, key("2"))
	out = m.View()
	assertLayout(t, out, w, h)
	t.Log("\nWorkflows tab:\n" + stripANSI(out))
}

func TestSortHelpAndSmall(t *testing.T) {
	const w, h = 90, 24
	m := New(darkTheme)
	m = step(t, m, tea.WindowSizeMsg{Width: w, Height: h})
	m = step(t, m, userLoadedMsg{login: "octocat"})
	m = step(t, m, reposLoadedMsg{repos: sampleRepos()})

	// Raise sort ribbon, then sort by "Updated" (by its first letter 'u').
	m = step(t, m, key("s"))
	if !m.repos.table.Sorting() {
		t.Fatal("expected sort ribbon to be raised")
	}
	out := m.View()
	assertLayout(t, out, w, h)
	t.Log("\nSort ribbon:\n" + stripANSI(out))

	m = step(t, m, key("u"))
	if m.repos.table.Sorting() {
		t.Fatal("ribbon should close after choosing a column")
	}
	out = m.View()
	if !strings.Contains(stripANSI(out), "Updated ↑") {
		t.Error("expected ascending arrow on Updated column")
	}
	assertLayout(t, out, w, h)

	// Help overlay.
	m = step(t, m, key("?"))
	if !m.showHelp {
		t.Fatal("expected help overlay open")
	}
	out = m.View()
	assertLayout(t, out, w, h)
	t.Log("\nHelp overlay:\n" + stripANSI(out))

	// Tiny terminal must not panic or overflow.
	m = step(t, m, key("?")) // close help
	for _, sz := range [][2]int{{80, 8}, {40, 6}, {120, 50}} {
		mm := step(t, m, tea.WindowSizeMsg{Width: sz[0], Height: sz[1]})
		assertLayout(t, mm.View(), sz[0], sz[1])
	}
}

func TestRunDetailRender(t *testing.T) {
	const w, h = 100, 30
	now := time.Now()
	base := now.Add(-time.Hour)

	m := New(darkTheme)
	m = step(t, m, tea.WindowSizeMsg{Width: w, Height: h})
	m = step(t, m, userLoadedMsg{login: "octocat"})
	m = step(t, m, reposLoadedMsg{repos: sampleRepos()})
	m = step(t, m, key("enter")) // open repo detail
	repo := m.detail.repo.NameWithOwner
	m = step(t, m, prsLoadedMsg{repo: repo, prs: nil})
	m = step(t, m, runsLoadedMsg{repo: repo, runs: []gh.Run{
		{DatabaseID: 111, WorkflowName: "Deploy", Status: "completed", Conclusion: "success",
			HeadBranch: "main", Event: "push", CreatedAt: base, URL: "https://x/run/111"},
	}})
	m = step(t, m, key("2")) // workflows tab

	// Enter returns a cmd that emits openRunMsg.
	nm, cmd := m.Update(key("enter"))
	m = nm.(Model)
	if cmd == nil {
		t.Fatal("expected a cmd from enter on a workflow run")
	}
	orm, ok := cmd().(openRunMsg)
	if !ok {
		t.Fatalf("expected openRunMsg, got %T", cmd())
	}
	m = step(t, m, orm)
	if m.screen != screenRunDetail {
		t.Fatalf("expected run-detail screen, got %v", m.screen)
	}

	m = step(t, m, runDetailLoadedMsg{repo: repo, runID: 111, detail: gh.RunDetail{
		Number: 219, Attempt: 1, WorkflowName: "Deploy", DisplayTitle: "fix: drop redundant prefix",
		Status: "completed", Conclusion: "success", HeadBranch: "main", HeadSha: "7f1eda5612abcdef",
		Event: "push", CreatedAt: base, UpdatedAt: base.Add(40 * time.Second), URL: "https://x/run/111",
		Jobs: []gh.Job{{
			Name: "build-and-deploy", Status: "completed", Conclusion: "success",
			StartedAt: base, CompletedAt: base.Add(36 * time.Second), URL: "https://x/job/1",
			Steps: []gh.Step{
				{Name: "Set up job", Number: 1, Status: "completed", Conclusion: "success", StartedAt: base, CompletedAt: base.Add(2 * time.Second)},
				{Name: "Deploy to Elastic Beanstalk", Number: 2, Status: "completed", Conclusion: "success", StartedAt: base.Add(2 * time.Second), CompletedAt: base.Add(36 * time.Second)},
			},
		}},
	}})

	out := m.View()
	assertLayout(t, out, w, h)
	t.Log("\nRun detail:\n" + stripANSI(out))

	if u := m.currentBrowserURL(); u != "https://x/run/111" {
		t.Errorf("ctrl+o url = %q, want run url", u)
	}

	// esc returns to the workflows tab.
	m = step(t, m, key("esc"))
	if m.screen != screenDetail {
		t.Fatalf("expected detail screen after esc, got %v", m.screen)
	}
}

func TestRefreshKey(t *testing.T) {
	const w, h = 90, 24
	now := time.Now()
	base := now.Add(-time.Hour)

	m := New(darkTheme)
	m = step(t, m, tea.WindowSizeMsg{Width: w, Height: h})
	m = step(t, m, userLoadedMsg{login: "octocat"})
	m = step(t, m, reposLoadedMsg{repos: sampleRepos()})

	// Repos screen: ctrl+f reloads.
	nm, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlF})
	m = nm.(Model)
	if !m.repos.loading {
		t.Error("repos should be loading after ctrl+f")
	}
	if cmd == nil {
		t.Error("expected refresh cmd on repos screen")
	}
	m = step(t, m, reposLoadedMsg{repos: sampleRepos()}) // settle

	// Detail screen.
	m = step(t, m, key("enter"))
	repo := m.detail.repo.NameWithOwner
	m = step(t, m, prsLoadedMsg{repo: repo, prs: nil})
	m = step(t, m, runsLoadedMsg{repo: repo, runs: []gh.Run{
		{DatabaseID: 7, WorkflowName: "CI", Status: "completed", Conclusion: "success", CreatedAt: base, URL: "https://x/7"},
	}})
	nm, cmd = m.Update(tea.KeyMsg{Type: tea.KeyCtrlF})
	m = nm.(Model)
	if !m.detail.loadingPRs || !m.detail.loadingRuns {
		t.Error("detail tabs should be loading after ctrl+f")
	}
	if cmd == nil {
		t.Error("expected refresh cmd on detail screen")
	}
	m = step(t, m, prsLoadedMsg{repo: repo, prs: nil})
	m = step(t, m, runsLoadedMsg{repo: repo, runs: []gh.Run{
		{DatabaseID: 7, WorkflowName: "CI", Status: "completed", Conclusion: "success", CreatedAt: base, URL: "https://x/7"},
	}})

	// Run-detail screen.
	m = step(t, m, key("2"))
	nm, cmd = m.Update(key("enter"))
	m = nm.(Model)
	m = step(t, m, cmd().(openRunMsg))
	m = step(t, m, runDetailLoadedMsg{repo: repo, runID: 7, detail: gh.RunDetail{Number: 1, URL: "https://x/7"}})
	nm, cmd = m.Update(tea.KeyMsg{Type: tea.KeyCtrlF})
	m = nm.(Model)
	if !m.runDetail.loading {
		t.Error("run detail should be loading after ctrl+f")
	}
	if cmd == nil {
		t.Error("expected refresh cmd on run-detail screen")
	}

	// ctrl+f must no longer page the table down.
	dt := NewDataTable([]Column{{Title: "A"}})
	dt.SetRows([][]string{{"1"}, {"2"}, {"3"}}, nil, nil)
	if _, consumed := dt.Update(tea.KeyMsg{Type: tea.KeyCtrlF}); consumed {
		t.Error("datatable should no longer consume ctrl+f")
	}
}

func openPRDetail(t *testing.T, w, h int) (Model, string) {
	t.Helper()
	now := time.Now()
	m := New(darkTheme)
	m = step(t, m, tea.WindowSizeMsg{Width: w, Height: h})
	m = step(t, m, userLoadedMsg{login: "octocat"})
	m = step(t, m, reposLoadedMsg{repos: sampleRepos()})
	m = step(t, m, key("enter"))
	repo := m.detail.repo.NameWithOwner
	m = step(t, m, prsLoadedMsg{repo: repo, prs: []gh.PR{
		{Number: 165, Title: "Fix the thing", HeadRefName: "fix/thing", UpdatedAt: now.Add(-2 * time.Hour), URL: "https://x/pr/165"},
	}})
	m = step(t, m, runsLoadedMsg{repo: repo, runs: nil})
	m = step(t, m, key("t")) // show all PRs (reviewer filter off)

	nm, cmd := m.Update(key("enter"))
	m = nm.(Model)
	if cmd == nil {
		t.Fatal("expected cmd from enter on a PR")
	}
	opr, ok := cmd().(openPRMsg)
	if !ok {
		t.Fatalf("expected openPRMsg, got %T", cmd())
	}
	m = step(t, m, opr)
	if m.screen != screenPRDetail {
		t.Fatalf("expected PR detail screen, got %v", m.screen)
	}
	return m, repo
}

func TestPRDetailRender(t *testing.T) {
	const w, h = 100, 30
	now := time.Now()
	m, repo := openPRDetail(t, w, h)

	m = step(t, m, prDetailLoadedMsg{repo: repo, number: 165, detail: gh.PRDetail{
		Number: 165, Title: "Fix the thing", State: "OPEN", Additions: 5, Deletions: 5, ChangedFiles: 3,
		BaseRefName: "master", HeadRefName: "fix/thing", ReviewDecision: "REVIEW_REQUIRED",
		CreatedAt: now.Add(-3 * time.Hour), UpdatedAt: now.Add(-2 * time.Hour), URL: "https://x/pr/165",
		Body: "This fixes the URL used for in-chapter downloads.",
		Files: []gh.ChangedFile{
			{Path: "Modules/Learning/x.blade.php", Additions: 2, Deletions: 2},
			{Path: "Modules/Learning/y.blade.php", Additions: 3, Deletions: 3},
		},
	}})

	out := m.View()
	assertLayout(t, out, w, h)
	t.Log("\nPR detail (info):\n" + stripANSI(out))

	if u := m.currentBrowserURL(); u != "https://x/pr/165" {
		t.Errorf("ctrl+o url = %q", u)
	}

	// ctrl+d -> diff view (triggers a diff load cmd).
	nm, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlD})
	m = nm.(Model)
	if m.prDetail.view != prViewDiff {
		t.Fatal("expected diff view after ctrl+d")
	}
	if cmd == nil {
		t.Fatal("expected diff load cmd")
	}
	m = step(t, m, prDiffLoadedMsg{repo: repo, number: 165, diff: "diff --git a/x b/x\n@@ -1 +1 @@\n-old\n+new\n"})
	out = m.View()
	assertLayout(t, out, w, h)
	t.Log("\nPR detail (diff):\n" + stripANSI(out))

	// ctrl+a -> approve confirmation prompt.
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlA})
	m = nm.(Model)
	if !m.prDetail.confirming() {
		t.Fatal("expected approve confirmation")
	}
	if !strings.Contains(stripANSI(m.View()), "Approve PR #165?") {
		t.Error("expected approve prompt text")
	}
	// 'y' confirms -> returns approve cmd.
	nm, cmd = m.Update(key("y"))
	m = nm.(Model)
	// cmd is a real gh mutation - assert it exists and state advanced, don't run it.
	if cmd == nil || m.prDetail.confirming() || !m.prDetail.working {
		t.Fatal("expected approve cmd, prompt cleared, and working set")
	}

	// Enter must NOT merge anymore; ctrl+y opens the merge prompt; 's' squashes.
	m = step(t, m, key("enter"))
	if m.prDetail.confirming() {
		t.Fatal("enter should not open the merge prompt")
	}
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlY})
	m = nm.(Model)
	if !m.prDetail.confirming() {
		t.Fatal("expected merge confirmation after ctrl+y")
	}
	nm, cmd = m.Update(key("s"))
	m = nm.(Model)
	if cmd == nil || m.prDetail.confirming() {
		t.Fatal("expected merge cmd and prompt cleared")
	}

	// ctrl+x -> close confirmation; 'y' closes.
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlX})
	m = nm.(Model)
	if !m.prDetail.confirming() {
		t.Fatal("expected close confirmation")
	}
	if !strings.Contains(stripANSI(m.View()), "Close PR #165") {
		t.Error("expected close prompt text")
	}
	nm, cmd = m.Update(key("y"))
	m = nm.(Model)
	if cmd == nil || m.prDetail.confirming() {
		t.Fatal("expected close cmd and prompt cleared")
	}

	// esc returns to the PRs tab.
	m = step(t, m, key("esc"))
	if m.screen != screenDetail {
		t.Fatalf("expected detail screen after esc, got %v", m.screen)
	}
}

func TestRunLogsAndActions(t *testing.T) {
	const w, h = 100, 30
	now := time.Now()
	base := now.Add(-30 * time.Minute)

	m := New(darkTheme)
	m = step(t, m, tea.WindowSizeMsg{Width: w, Height: h})
	m = step(t, m, userLoadedMsg{login: "octocat"})
	m = step(t, m, reposLoadedMsg{repos: sampleRepos()})
	m = step(t, m, key("enter"))
	repo := m.detail.repo.NameWithOwner
	m = step(t, m, prsLoadedMsg{repo: repo, prs: nil})
	m = step(t, m, runsLoadedMsg{repo: repo, runs: []gh.Run{
		{DatabaseID: 555, WorkflowName: "Deploy", Status: "in_progress", HeadBranch: "main", Event: "push", CreatedAt: base, URL: "https://x/run/555"},
	}})
	m = step(t, m, key("2"))
	nm, cmd := m.Update(key("enter"))
	m = nm.(Model)
	m = step(t, m, cmd().(openRunMsg))
	m = step(t, m, runDetailLoadedMsg{repo: repo, runID: 555, detail: gh.RunDetail{
		Number: 12, WorkflowName: "Deploy", Status: "in_progress", HeadBranch: "main", Event: "push",
		HeadSha: "abcdef1234", CreatedAt: base, UpdatedAt: now, URL: "https://x/run/555",
		Jobs: []gh.Job{{
			DatabaseID: 555, Name: "build-and-deploy", Status: "in_progress",
			StartedAt: base, URL: "https://x/job/555",
			Steps: []gh.Step{{Name: "Set up job", Number: 1, Status: "completed", Conclusion: "success", StartedAt: base, CompletedAt: base.Add(3 * time.Second)}},
		}},
	}})

	// enter -> open logs (fires a log-load cmd).
	nm, cmd = m.Update(key("enter"))
	m = nm.(Model)
	if m.runDetail.view != runViewLogs {
		t.Fatal("expected logs view after enter")
	}
	if cmd == nil {
		t.Fatal("expected a log-load cmd")
	}
	// Don't execute cmd here - it would hit the network. Simulate the result.
	m = step(t, m, runLogLoadedMsg{repo: repo, runID: 555, jobID: 555,
		log: "2026-06-03T01:13:40Z Set up job\n2026-06-03T01:13:42Z " + strings.Repeat("x", 250) + "\nDone"})
	out := m.View()
	assertLayout(t, out, w, h)
	t.Log("\nRun logs:\n" + stripANSI(out))

	// esc pops logs -> jobs (still on run detail).
	m = step(t, m, key("esc"))
	if m.runDetail.view != runViewJobs || m.screen != screenRunDetail {
		t.Fatalf("expected jobs view on run detail after esc; view=%v screen=%v", m.runDetail.view, m.screen)
	}

	// ctrl+r -> rerun prompt; 'f' reruns failed.
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlR})
	m = nm.(Model)
	if !m.runDetail.confirming() {
		t.Fatal("expected rerun confirmation")
	}
	if !strings.Contains(stripANSI(m.View()), "Re-run this run?") {
		t.Error("expected rerun prompt text")
	}
	nm, cmd = m.Update(key("f"))
	m = nm.(Model)
	// cmd is a real gh mutation - assert it exists and state advanced, don't run it.
	if cmd == nil || m.runDetail.confirming() || !m.runDetail.working {
		t.Fatal("expected rerun cmd, prompt cleared, and working set")
	}

	// ctrl+x -> cancel prompt (allowed: run is in_progress); 'y' cancels.
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlX})
	m = nm.(Model)
	if !m.runDetail.confirming() {
		t.Fatal("expected cancel confirmation")
	}
	nm, cmd = m.Update(key("y"))
	m = nm.(Model)
	if cmd == nil || m.runDetail.confirming() {
		t.Fatal("expected cancel cmd and prompt cleared")
	}

	// esc returns to workflows tab.
	m = step(t, m, key("esc"))
	if m.screen != screenDetail {
		t.Fatalf("expected detail screen after esc, got %v", m.screen)
	}
}

func TestAutoRefresh(t *testing.T) {
	const w, h = 90, 24
	now := time.Now()

	m := New(darkTheme)
	m = step(t, m, tea.WindowSizeMsg{Width: w, Height: h})
	m = step(t, m, userLoadedMsg{login: "octocat"})
	m = step(t, m, reposLoadedMsg{repos: sampleRepos()})

	// On the repos screen, a tick should NOT trigger a refresh (only reschedule).
	if cmds := m.autoRefreshCmds(); cmds != nil {
		t.Error("repos screen should not auto-refresh")
	}

	m = step(t, m, key("enter"))
	repo := m.detail.repo.NameWithOwner
	m = step(t, m, prsLoadedMsg{repo: repo, prs: nil})

	// Workflows tab with an in-progress run -> auto-refresh is live.
	m = step(t, m, runsLoadedMsg{repo: repo, runs: []gh.Run{
		{DatabaseID: 1, WorkflowName: "Deploy", Status: "in_progress", CreatedAt: now, URL: "https://x/1"},
	}})
	m = step(t, m, key("2")) // workflows tab
	if !m.detail.snapshot().Live {
		t.Error("workflows tab with in-progress run should be live")
	}
	if len(m.autoRefreshCmds()) != 1 {
		t.Error("expected a runs reload cmd while a run is in progress")
	}
	// A tick reschedules itself (always returns a cmd).
	_, cmd := m.Update(autoRefreshTickMsg{})
	if cmd == nil {
		t.Error("tick should reschedule")
	}

	// All runs completed -> no longer live, no refresh.
	m = step(t, m, runsLoadedMsg{repo: repo, runs: []gh.Run{
		{DatabaseID: 1, WorkflowName: "Deploy", Status: "completed", Conclusion: "success", CreatedAt: now, URL: "https://x/1"},
	}})
	if m.detail.snapshot().Live {
		t.Error("completed runs should not be live")
	}
	if m.autoRefreshCmds() != nil {
		t.Error("completed runs should not auto-refresh")
	}

	// Run detail of an in-progress run is live; completed is not.
	m = step(t, m, openRunMsg{repo: repo, run: gh.Run{DatabaseID: 9, URL: "https://x/9"}})
	m = step(t, m, runDetailLoadedMsg{repo: repo, runID: 9, detail: gh.RunDetail{Number: 9, Status: "in_progress", URL: "https://x/9"}})
	if !m.runDetail.snapshot().Live || len(m.autoRefreshCmds()) != 1 {
		t.Error("in-progress run detail should auto-refresh and be live")
	}
	if !strings.Contains(stripANSI(m.View()), "● live") {
		t.Error("expected live indicator in footer")
	}
	m = step(t, m, runDetailLoadedMsg{repo: repo, runID: 9, detail: gh.RunDetail{Number: 9, Status: "completed", Conclusion: "success", URL: "https://x/9"}})
	if m.runDetail.snapshot().Live || m.autoRefreshCmds() != nil {
		t.Error("completed run detail should not auto-refresh")
	}
}

func TestPRReviewCompose(t *testing.T) {
	const w, h = 100, 30
	m, repo := openPRDetail(t, w, h)
	m = step(t, m, prDetailLoadedMsg{repo: repo, number: 165, detail: gh.PRDetail{
		Number: 165, Title: "Fix the thing", State: "OPEN", BaseRefName: "master", HeadRefName: "fix",
	}})

	// ctrl+r opens the composer (choosing stage).
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlR})
	m = nm.(Model)
	if !m.prDetail.composing || m.prDetail.composeTyping {
		t.Fatal("expected composer in choosing stage")
	}
	if !strings.Contains(stripANSI(m.View()), "request changes") {
		t.Error("expected review kind prompt")
	}

	// 'r' selects request-changes and moves to typing.
	m = step(t, m, key("r"))
	if !m.prDetail.composeTyping || m.prDetail.composeKind != gh.ReviewRequestChanges {
		t.Fatal("expected typing stage with request-changes kind")
	}

	// Empty body: enter does nothing (still composing).
	nm, cmd := m.Update(key("enter"))
	m = nm.(Model)
	if cmd != nil || !m.prDetail.composing {
		t.Fatal("empty body should not submit")
	}

	// Type a body, then submit.
	m = step(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("please fix the URL")})
	if got := m.prDetail.input.Value(); got != "please fix the URL" {
		t.Fatalf("input value = %q", got)
	}
	nm, cmd = m.Update(key("enter"))
	m = nm.(Model)
	if cmd == nil || m.prDetail.composing || !m.prDetail.working {
		t.Fatal("expected submit cmd, composer closed, working set")
	}

	// esc cancels a composer mid-flight.
	m = step(t, m, tea.KeyMsg{Type: tea.KeyCtrlR})
	if !m.prDetail.composing {
		t.Fatal("expected composer open")
	}
	m = step(t, m, key("esc"))
	if m.prDetail.composing || m.screen != screenPRDetail {
		t.Fatal("esc should cancel composer without leaving the screen")
	}
}

func TestPRMentionPicker(t *testing.T) {
	const w, h = 100, 30
	m, repo := openPRDetail(t, w, h)
	m = step(t, m, prDetailLoadedMsg{repo: repo, number: 165, detail: gh.PRDetail{
		Number: 165, Title: "Fix the thing", State: "OPEN", BaseRefName: "master", HeadRefName: "fix",
	}})
	// Repo mentionable users arrive from the lazy fetch.
	m = step(t, m, mentionUsersLoadedMsg{repo: repo, logins: []string{"alice", "bob", "alfred"}})

	// Open the composer and choose "comment".
	m = step(t, m, tea.KeyMsg{Type: tea.KeyCtrlR})
	m = step(t, m, key("c"))
	if !m.prDetail.composeTyping {
		t.Fatal("expected typing stage")
	}

	// Typing "@al" opens the picker, matching alice + alfred by prefix.
	m = step(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("@al")})
	if !m.prDetail.mentioning {
		t.Fatal("expected mention picker open after typing @al")
	}
	if got := m.prDetail.mentionMatches; len(got) != 2 || got[0] != "alice" || got[1] != "alfred" {
		t.Fatalf("matches = %v, want [alice alfred]", got)
	}
	if !strings.Contains(stripANSI(m.View()), "@alfred") {
		t.Error("expected candidate list rendered in body")
	}

	// Down selects alfred; enter inserts it and closes the picker.
	m = step(t, m, tea.KeyMsg{Type: tea.KeyDown})
	m = step(t, m, key("enter"))
	if m.prDetail.mentioning {
		t.Fatal("picker should close after accepting")
	}
	if got := m.prDetail.input.Value(); got != "@alfred " {
		t.Fatalf("input = %q, want %q", got, "@alfred ")
	}

	// With the picker closed, enter now submits the review.
	m = step(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("thanks")})
	nm, cmd := m.Update(key("enter"))
	m = nm.(Model)
	if cmd == nil || m.prDetail.composing {
		t.Fatal("expected enter to submit once the picker is closed")
	}
}

func TestMyPRsDashboard(t *testing.T) {
	const w, h = 110, 30
	now := time.Now()

	m := New(darkTheme)
	m = step(t, m, tea.WindowSizeMsg{Width: w, Height: h})
	m = step(t, m, userLoadedMsg{login: "octocat"})
	m = step(t, m, reposLoadedMsg{repos: sampleRepos()})

	// 'p' opens the dashboard and kicks off a load.
	nm, cmd := m.Update(key("p"))
	m = nm.(Model)
	if m.screen != screenMyPRs || !m.myPRs.loading {
		t.Fatal("expected My PRs screen loading after 'p'")
	}
	if cmd == nil {
		t.Fatal("expected a load cmd")
	}

	m = step(t, m, myPRsLoadedMsg{
		review: []gh.SearchPR{
			{Number: 7, Title: "Fix the URL", Repository: gh.SearchRepo{NameWithOwner: "acme/widget-api"}, URL: "https://x/pr/7", UpdatedAt: now.Add(-time.Hour)},
		},
		authored: []gh.SearchPR{
			{Number: 9, Title: "My feature", Repository: gh.SearchRepo{NameWithOwner: "acme/auth-api"}, URL: "https://x/pr/9", UpdatedAt: now.Add(-2 * time.Hour)},
		},
	})

	out := m.View()
	assertLayout(t, out, w, h)
	t.Log("\nMy PRs (reviews only):\n" + stripANSI(out))
	plain := stripANSI(out)
	if !strings.Contains(plain, "widget-api") || strings.Contains(plain, "auth-api") {
		t.Error("reviews-only should show review PR, hide authored")
	}
	if m.myPRs.shownCount() != 1 {
		t.Errorf("expected 1 shown, got %d", m.myPRs.shownCount())
	}

	// 't' includes authored PRs.
	m = step(t, m, key("t"))
	plain = stripANSI(m.View())
	if !strings.Contains(plain, "auth-api") {
		t.Error("expected authored PR after toggle")
	}
	if m.myPRs.shownCount() != 2 {
		t.Errorf("expected 2 shown, got %d", m.myPRs.shownCount())
	}

	// ctrl+o resolves the selected PR's URL.
	if u := m.currentBrowserURL(); u == "" {
		t.Error("expected a browser url for the selected PR")
	}

	// enter drills into PR detail; esc returns to the dashboard (not repos).
	nm, cmd = m.Update(key("enter"))
	m = nm.(Model)
	opr, ok := cmd().(openPRMsg)
	if !ok {
		t.Fatalf("expected openPRMsg, got %T", cmd())
	}
	m = step(t, m, opr)
	if m.screen != screenPRDetail {
		t.Fatal("expected PR detail screen")
	}
	m = step(t, m, key("esc"))
	if m.screen != screenMyPRs {
		t.Fatalf("esc from PR detail should return to My PRs, got %v", m.screen)
	}

	// esc from the dashboard returns to repositories.
	m = step(t, m, key("esc"))
	if m.screen != screenRepos {
		t.Fatalf("expected repos after esc, got %v", m.screen)
	}
}

func TestRepoCacheOrdering(t *testing.T) {
	const w, h = 90, 24
	cached := []gh.Repo{{Name: "old", NameWithOwner: "me/old-cached", PushedAt: time.Now().Add(-time.Hour)}}
	fresh := sampleRepos()

	// Cache arrives first: shown immediately, marked refreshing.
	m := New(darkTheme)
	m = step(t, m, tea.WindowSizeMsg{Width: w, Height: h})
	m = step(t, m, reposCacheLoadedMsg{repos: cached, savedAt: time.Now().Add(-10 * time.Minute)})
	if m.repos.loading {
		t.Error("cache should clear loading")
	}
	if !m.repos.refreshing || m.repos.fresh {
		t.Error("should be refreshing (not fresh) while showing cache")
	}
	if len(m.repos.repos) != 1 {
		t.Fatalf("expected 1 cached repo, got %d", len(m.repos.repos))
	}
	if msg := m.repos.snapshot().Message; msg != "refreshing…" {
		t.Errorf("expected refreshing message, got %q", msg)
	}

	// Network arrives: replaces cache, clears refreshing.
	m = step(t, m, reposLoadedMsg{repos: fresh})
	if m.repos.refreshing || !m.repos.fresh {
		t.Error("fresh data should clear refreshing and set fresh")
	}
	if len(m.repos.repos) != len(fresh) {
		t.Errorf("expected %d fresh repos, got %d", len(fresh), len(m.repos.repos))
	}

	// A late cache read must NOT overwrite fresh data.
	m = step(t, m, reposCacheLoadedMsg{repos: cached, savedAt: time.Now()})
	if len(m.repos.repos) != len(fresh) || m.repos.refreshing {
		t.Error("late cache must not clobber fresh data")
	}

	// No-cache path: loading stays until the network responds.
	m2 := New(darkTheme)
	m2 = step(t, m2, tea.WindowSizeMsg{Width: w, Height: h})
	m2 = step(t, m2, reposCacheLoadedMsg{}) // empty (cache miss)
	if !m2.repos.loading {
		t.Error("cache miss should keep loading until network responds")
	}
}

func TestPRConversationView(t *testing.T) {
	const w, h = 100, 30
	t1 := time.Now().Add(-3 * time.Hour)
	t2 := time.Now().Add(-1 * time.Hour)

	m, repo := openPRDetail(t, w, h)
	m = step(t, m, prDetailLoadedMsg{repo: repo, number: 165, detail: gh.PRDetail{
		Number: 165, Title: "Fix the thing", State: "OPEN", BaseRefName: "master", HeadRefName: "fix",
		Reviews:  []gh.Review{{State: "APPROVED", Body: "looks good to me", SubmittedAt: t2}},
		Comments: []gh.Comment{{Body: "please rebase first", CreatedAt: t1}},
	}})

	// tab: Info -> Diff (lazy diff load fires).
	nm, cmd := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = nm.(Model)
	if m.prDetail.view != prViewDiff {
		t.Fatalf("expected diff view, got %v", m.prDetail.view)
	}
	if cmd == nil {
		t.Error("expected lazy diff-load cmd")
	}
	m = step(t, m, prDiffLoadedMsg{repo: repo, number: 165, diff: "diff --git a/x b/x\n@@ -1 +1 @@\n-a\n+b\n"})

	// tab: Diff -> Conversation.
	m = step(t, m, tea.KeyMsg{Type: tea.KeyTab})
	if m.prDetail.view != prViewConversation {
		t.Fatalf("expected conversation view, got %v", m.prDetail.view)
	}
	out := m.View()
	assertLayout(t, out, w, h)
	t.Log("\nPR conversation:\n" + stripANSI(out))

	plain := stripANSI(out)
	iRebase := strings.Index(plain, "please rebase first")
	iGood := strings.Index(plain, "looks good to me")
	if iRebase < 0 || iGood < 0 {
		t.Fatal("expected both comment and review bodies in conversation")
	}
	if iRebase > iGood {
		t.Error("conversation should be chronological (older comment before newer review)")
	}

	// tab wraps back to Info.
	m = step(t, m, tea.KeyMsg{Type: tea.KeyTab})
	if m.prDetail.view != prViewInfo {
		t.Fatalf("expected wrap back to info, got %v", m.prDetail.view)
	}
}

func sampleNotifs() []gh.Notification {
	mk := func(id, repo, typ, title, url string, ago time.Duration) gh.Notification {
		var n gh.Notification
		n.ID = id
		n.Unread = true
		n.Reason = "review_requested"
		n.UpdatedAt = time.Now().Add(-ago)
		n.Repository.FullName = repo
		n.Subject.Title = title
		n.Subject.Type = typ
		n.Subject.URL = url
		return n
	}
	return []gh.Notification{
		mk("111", "acme/payments-api", "PullRequest", "Phase 1 schema", "https://api.github.com/repos/acme/payments-api/pulls/848", time.Hour),
		mk("222", "acme/auth-api", "Issue", "Bug: crash on save", "https://api.github.com/repos/acme/auth-api/issues/12", 2*time.Hour),
	}
}

func TestNotificationsInbox(t *testing.T) {
	const w, h = 110, 24
	m := New(darkTheme)
	m = step(t, m, tea.WindowSizeMsg{Width: w, Height: h})
	m = step(t, m, userLoadedMsg{login: "octocat"})
	m = step(t, m, reposLoadedMsg{repos: sampleRepos()})

	// 'n' opens the inbox and loads.
	nm, cmd := m.Update(key("n"))
	m = nm.(Model)
	if m.screen != screenNotifs || !m.notifs.loading || cmd == nil {
		t.Fatal("expected notifications screen loading after 'n'")
	}
	m = step(t, m, notifsLoadedMsg{notifs: sampleNotifs()})
	out := m.View()
	assertLayout(t, out, w, h)
	t.Log("\nNotifications:\n" + stripANSI(out))

	// WebURL derivation for a PR notification.
	if n, _ := m.notifs.selected(); n.WebURL() != "https://github.com/acme/payments-api/pull/848" {
		t.Errorf("PR web url = %q", n.WebURL())
	}

	// enter on a NON-PR notification (the Issue, row 2) is a no-op; ctrl+o opens.
	m = step(t, m, key("j")) // move to the issue notification
	if _, ec := m.Update(key("enter")); ec != nil {
		t.Error("enter should be a no-op on a non-PR notification")
	}
	if u := m.currentBrowserURL(); u == "" {
		t.Error("ctrl+o should still resolve a URL for a non-PR notification")
	}
	m = step(t, m, key("k")) // back to the PR notification

	// enter on a PR notification drills into PR detail (return = notifs).
	nm, cmd = m.Update(key("enter"))
	m = nm.(Model)
	opr, ok := cmd().(openPRMsg)
	if !ok || opr.pr.Number != 848 {
		t.Fatalf("expected openPRMsg #848, got %#v", cmd())
	}
	m = step(t, m, opr)
	m = step(t, m, key("esc"))
	if m.screen != screenNotifs {
		t.Fatalf("esc should return to notifications, got %v", m.screen)
	}

	// 'x' marks read: removes optimistically and fires a cmd.
	before := len(m.notifs.notifs)
	nm, cmd = m.Update(key("x"))
	m = nm.(Model)
	if len(m.notifs.notifs) != before-1 || cmd == nil {
		t.Fatal("expected optimistic removal + mark-read cmd")
	}

	// esc back to repos.
	m = step(t, m, key("esc"))
	if m.screen != screenRepos {
		t.Fatalf("expected repos, got %v", m.screen)
	}
}

func TestIssuesTabAndDetail(t *testing.T) {
	const w, h = 110, 28
	now := time.Now()
	m := New(darkTheme)
	m = step(t, m, tea.WindowSizeMsg{Width: w, Height: h})
	m = step(t, m, userLoadedMsg{login: "octocat"})
	m = step(t, m, reposLoadedMsg{repos: sampleRepos()})
	m = step(t, m, key("enter"))
	repo := m.detail.repo.NameWithOwner
	m = step(t, m, prsLoadedMsg{repo: repo, prs: nil})
	m = step(t, m, runsLoadedMsg{repo: repo, runs: nil})
	m = step(t, m, issuesLoadedMsg{repo: repo, issues: []gh.Issue{
		{Number: 106, Title: "Support multiple parameter paths", State: "OPEN",
			Labels: []gh.Label{{Name: "enhancement"}}, UpdatedAt: now.Add(-2 * time.Hour), URL: "https://x/issues/106"},
	}})

	// Switch to Issues tab (3).
	m = step(t, m, key("3"))
	out := m.View()
	assertLayout(t, out, w, h)
	t.Log("\nIssues tab:\n" + stripANSI(out))
	if !strings.Contains(stripANSI(out), "Support multiple parameter paths") {
		t.Error("expected issue row")
	}

	// enter -> issue detail.
	nm, cmd := m.Update(key("enter"))
	m = nm.(Model)
	opi, ok := cmd().(openIssueMsg)
	if !ok || opi.issue.Number != 106 {
		t.Fatalf("expected openIssueMsg #106, got %#v", cmd())
	}
	m = step(t, m, opi)
	m = step(t, m, issueDetailLoadedMsg{repo: repo, number: 106, detail: gh.IssueDetail{
		Number: 106, Title: "Support multiple parameter paths", State: "OPEN", Body: "We need comma-delimited paths.",
		Labels: []gh.Label{{Name: "enhancement"}}, CreatedAt: now.Add(-3 * time.Hour), UpdatedAt: now.Add(-2 * time.Hour),
		Comments: []gh.Comment{{Body: "Working on it", CreatedAt: now.Add(-time.Hour)}}, URL: "https://x/issues/106",
	}})
	out = m.View()
	assertLayout(t, out, w, h)
	t.Log("\nIssue detail:\n" + stripANSI(out))
	if !strings.Contains(stripANSI(out), "comma-delimited") || !strings.Contains(stripANSI(out), "Working on it") {
		t.Error("expected body + comment in issue detail")
	}

	// ctrl+r comment composer -> type -> submit.
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlR})
	m = nm.(Model)
	if !m.issueDetail.composing {
		t.Fatal("expected comment composer")
	}
	m = step(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("on it now")})
	nm, cmd = m.Update(key("enter"))
	m = nm.(Model)
	if cmd == nil || m.issueDetail.composing || !m.issueDetail.working {
		t.Fatal("expected comment submit cmd, composer closed, working set")
	}

	// ctrl+x close -> confirm prompt -> y.
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlX})
	m = nm.(Model)
	if !m.issueDetail.pendingClose {
		t.Fatal("expected close confirmation")
	}
	nm, cmd = m.Update(key("y"))
	m = nm.(Model)
	if cmd == nil || m.issueDetail.pendingClose {
		t.Fatal("expected close cmd and prompt cleared")
	}

	// esc back to the Issues tab.
	m = step(t, m, key("esc"))
	if m.screen != screenDetail || m.detail.active != tabIssues {
		t.Fatalf("esc should return to issues tab; screen=%v tab=%v", m.screen, m.detail.active)
	}
}

func notif(id, reason, typ string) gh.Notification {
	var n gh.Notification
	n.ID = id
	n.Unread = true
	n.Reason = reason
	n.UpdatedAt = time.Now()
	n.Repository.FullName = "org/repo"
	n.Subject.Title = "t-" + id
	n.Subject.Type = typ
	n.Subject.URL = "https://api.github.com/repos/org/repo/pulls/" + id
	return n
}

func TestNotifsPowerUps(t *testing.T) {
	const w, h = 110, 24
	m := New(darkTheme)
	m = step(t, m, tea.WindowSizeMsg{Width: w, Height: h})
	m = step(t, m, userLoadedMsg{login: "octocat"})
	m = step(t, m, reposLoadedMsg{repos: sampleRepos()})
	nm, _ := m.Update(key("n"))
	m = nm.(Model)
	m = step(t, m, notifsLoadedMsg{page: 1, notifs: []gh.Notification{
		notif("1", "review_requested", "PullRequest"),
		notif("2", "mention", "Issue"),
		notif("3", "ci_activity", "CheckSuite"),
	}})
	if m.notifs.shownCount() != 3 {
		t.Fatalf("expected 3, got %d", m.notifs.shownCount())
	}

	// 'f' cycles filter to review_requested -> 1 shown.
	m = step(t, m, key("f"))
	if m.notifs.reasonFilter != "review_requested" || m.notifs.shownCount() != 1 {
		t.Fatalf("filter=%q shown=%d", m.notifs.reasonFilter, m.notifs.shownCount())
	}
	m = step(t, m, key("f")) // -> mention
	if m.notifs.reasonFilter != "mention" || m.notifs.shownCount() != 1 {
		t.Fatalf("filter=%q shown=%d", m.notifs.reasonFilter, m.notifs.shownCount())
	}

	// 'm' load more -> fires page 2 cmd; feed it -> appended (filter still mention).
	nm, cmd := m.Update(key("m"))
	m = nm.(Model)
	if cmd == nil || !m.notifs.loadingMore {
		t.Fatal("expected load-more cmd")
	}
	m = step(t, m, notifsLoadedMsg{page: 2, notifs: []gh.Notification{notif("4", "mention", "Issue")}})
	if len(m.notifs.notifs) != 4 || m.notifs.page != 2 || m.notifs.loadingMore {
		t.Fatalf("after load more: total=%d page=%d", len(m.notifs.notifs), m.notifs.page)
	}
	if m.notifs.shownCount() != 2 { // two 'mention' now
		t.Errorf("expected 2 mentions shown, got %d", m.notifs.shownCount())
	}

	// 'A' -> confirm prompt -> 'y' fires mark-all; feed success clears list.
	m = step(t, m, key("A"))
	if !m.notifs.pendingMarkAll {
		t.Fatal("expected mark-all confirmation")
	}
	nm, cmd = m.Update(key("y"))
	m = nm.(Model)
	if cmd == nil || m.notifs.pendingMarkAll {
		t.Fatal("expected mark-all cmd and prompt cleared")
	}
	m = step(t, m, notifsMarkedAllMsg{err: nil})
	if len(m.notifs.notifs) != 0 {
		t.Errorf("mark-all should clear the list, got %d", len(m.notifs.notifs))
	}
}

func TestCommandPalette(t *testing.T) {
	const w, h = 100, 24
	m := New(darkTheme)
	m = step(t, m, tea.WindowSizeMsg{Width: w, Height: h})
	m = step(t, m, userLoadedMsg{login: "octocat"})
	m = step(t, m, reposLoadedMsg{repos: sampleRepos()})

	// ctrl+k opens the palette.
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlK})
	m = nm.(Model)
	if !m.palette.active {
		t.Fatal("ctrl+k should open the palette")
	}
	out := m.View()
	assertLayout(t, out, w, h)
	if !strings.Contains(stripANSI(out), "Go to") {
		t.Error("expected palette overlay")
	}

	// Type to fuzzy-filter to the hello-world repo.
	m = step(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("hello")})
	sel, ok := m.palette.selected()
	if !ok || sel.kind != paletteRepo || sel.repo.NameWithOwner != "octocat/hello-world" {
		t.Fatalf("expected hello-world repo selected, got %#v", sel)
	}
	t.Log("\nPalette:\n" + stripANSI(m.View()))

	// enter -> opens that repo's detail.
	nm, cmd := m.Update(key("enter"))
	m = nm.(Model)
	if m.palette.active {
		t.Error("palette should close on enter")
	}
	if m.screen != screenDetail || m.detail.repo.NameWithOwner != "octocat/hello-world" {
		t.Fatalf("expected framework detail, got screen=%v repo=%q", m.screen, m.detail.repo.NameWithOwner)
	}
	if cmd == nil {
		t.Error("expected detail load cmd")
	}

	// Reopen, jump to a screen target (My PRs).
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlK})
	m = nm.(Model)
	m = step(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("my pr")})
	nm, _ = m.Update(key("enter"))
	m = nm.(Model)
	if m.screen != screenMyPRs {
		t.Fatalf("expected My PRs screen, got %v", m.screen)
	}

	// esc closes without navigating.
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlK})
	m = nm.(Model)
	m = step(t, m, key("esc"))
	if m.palette.active {
		t.Error("esc should close the palette")
	}
}

func TestTableFilterEverywhere(t *testing.T) {
	const w, h = 100, 24
	now := time.Now()
	m := New(darkTheme)
	m = step(t, m, tea.WindowSizeMsg{Width: w, Height: h})
	m = step(t, m, userLoadedMsg{login: "octocat"})
	m = step(t, m, reposLoadedMsg{repos: sampleRepos()})

	// Repos: '/' filters the table.
	m = step(t, m, key("/"))
	if !m.repos.table.Filtering() {
		t.Fatal("'/' should start filtering the repo table")
	}
	m = step(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("widget")})
	if m.repos.table.Len() != 1 {
		t.Fatalf("expected 1 filtered repo, got %d", m.repos.table.Len())
	}
	if r, ok := m.repos.selected(); !ok || r.NameWithOwner != "acme/widget-api" {
		t.Fatalf("filtered selection wrong: %#v", r)
	}
	// '?' should type into the filter, not open help.
	if m.showHelp {
		t.Fatal("help should not be open while filtering")
	}
	m = step(t, m, key("esc")) // clear filter
	if m.repos.table.Filtering() || m.repos.table.Len() != 3 {
		t.Fatalf("esc should clear filter; filtering=%v len=%d", m.repos.table.Filtering(), m.repos.table.Len())
	}

	// Workflows tab: '/' filters runs too.
	m = step(t, m, key("enter"))
	repo := m.detail.repo.NameWithOwner
	m = step(t, m, prsLoadedMsg{repo: repo, prs: nil})
	m = step(t, m, runsLoadedMsg{repo: repo, runs: []gh.Run{
		{DatabaseID: 1, WorkflowName: "CI", Status: "completed", Conclusion: "success", CreatedAt: now, URL: "https://x/1"},
		{DatabaseID: 2, WorkflowName: "Deploy", Status: "completed", Conclusion: "success", CreatedAt: now, URL: "https://x/2"},
	}})
	m = step(t, m, key("2")) // workflows tab
	m = step(t, m, key("/"))
	if !m.detail.activeTable().Filtering() {
		t.Fatal("'/' should filter the workflows table")
	}
	m = step(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("Deploy")})
	if m.detail.runTable.Len() != 1 {
		t.Fatalf("expected 1 filtered run, got %d", m.detail.runTable.Len())
	}
	out := m.View()
	assertLayout(t, out, w, h)
	// '3' must type into the filter, not switch to Issues tab.
	if m.detail.active != tabWorkflows {
		t.Fatal("tab must not switch while filtering")
	}
	m = step(t, m, key("esc"))
	if m.detail.runTable.Len() != 2 {
		t.Fatalf("esc should clear run filter, got %d", m.detail.runTable.Len())
	}
}

func TestSecurityTab(t *testing.T) {
	const w, h = 110, 26
	now := time.Now()
	mk := func(source, sev, detail, location, url string) gh.SecurityAlert {
		return gh.SecurityAlert{
			Source: source, Severity: sev, Detail: detail, Location: location,
			HTMLURL: url, CreatedAt: now.Add(-24 * time.Hour),
		}
	}

	openDetail := func() Model {
		m := New(darkTheme)
		m = step(t, m, tea.WindowSizeMsg{Width: w, Height: h})
		m = step(t, m, userLoadedMsg{login: "octocat"})
		m = step(t, m, reposLoadedMsg{repos: sampleRepos()})
		m = step(t, m, key("enter"))
		repo := m.detail.repo.NameWithOwner
		m = step(t, m, prsLoadedMsg{repo: repo, prs: nil})
		m = step(t, m, runsLoadedMsg{repo: repo, runs: nil})
		m = step(t, m, issuesLoadedMsg{repo: repo, issues: nil})
		return m
	}

	// Alerts present.
	m := openDetail()
	repo := m.detail.repo.NameWithOwner
	// RepoSecurityAlerts returns these severity-sorted (high first).
	m = step(t, m, securityLoadedMsg{repo: repo, alerts: []gh.SecurityAlert{
		mk("dependabot", "high", "SSRF via redirect", "guzzlehttp/guzzle", "https://x/security/190"),
		mk("dependabot", "low", "IDN punycode handling", "symfony/polyfill-intl-idn", "https://x/security/184"),
	}})
	m = step(t, m, key("4")) // Security tab
	out := m.View()
	assertLayout(t, out, w, h)
	t.Log("\nSecurity tab:\n" + stripANSI(out))
	plain := stripANSI(out)
	if !strings.Contains(plain, "guzzlehttp/guzzle") || !strings.Contains(plain, "high") {
		t.Error("expected alert rows")
	}
	// high should sort above low (pre-sorted by severity) - compare the unique
	// package names to avoid matching "low" inside "Workflows".
	if i, j := strings.Index(plain, "guzzlehttp/guzzle"), strings.Index(plain, "symfony/polyfill"); i < 0 || j < 0 || i > j {
		t.Error("expected high-severity row listed above low-severity row")
	}
	// ctrl+o (consistent open-in-browser) resolves the selected row's advisory.
	if u := m.currentBrowserURL(); u != "https://x/security/190" {
		t.Errorf("ctrl+o url = %q", u)
	}
	nm, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlO})
	m = nm.(Model)
	if cmd == nil {
		t.Error("ctrl+o should open the advisory")
	}
	// enter must NOT open the browser on the Security tab (no drill-in screen).
	if _, ec := m.Update(key("enter")); ec != nil {
		t.Error("enter should be a no-op on the Security tab")
	}

	// All sources unavailable -> disabled state.
	m2 := openDetail()
	m2 = step(t, m2, securityLoadedMsg{repo: m2.detail.repo.NameWithOwner,
		unavailable: []string{"Dependabot", "code scanning", "secret scanning"}})
	m2 = step(t, m2, key("4"))
	out2 := m2.View()
	assertLayout(t, out2, w, h)
	if !strings.Contains(stripANSI(out2), "disabled or inaccessible") {
		t.Error("expected disabled message")
	}

	// Partial availability -> shows alerts + notes the unavailable sources.
	m3 := openDetail()
	m3 = step(t, m3, securityLoadedMsg{repo: m3.detail.repo.NameWithOwner,
		alerts:      []gh.SecurityAlert{mk("secret", "high", "AWS Access Key", "", "https://x/secret/1")},
		unavailable: []string{"code scanning"}})
	m3 = step(t, m3, key("4"))
	out3 := stripANSI(m3.View())
	assertLayout(t, m3.View(), w, h)
	if !strings.Contains(out3, "secret") || !strings.Contains(out3, "code scanning unavailable") {
		t.Error("expected secret alert shown and code scanning noted unavailable")
	}
}

func TestSecurityManyAlerts(t *testing.T) {
	const w, h = 110, 20 // small height: most rows are off-screen and must scroll
	now := time.Now()
	m := New(darkTheme)
	m = step(t, m, tea.WindowSizeMsg{Width: w, Height: h})
	m = step(t, m, userLoadedMsg{login: "octocat"})
	m = step(t, m, reposLoadedMsg{repos: sampleRepos()})
	m = step(t, m, key("enter"))
	repo := m.detail.repo.NameWithOwner
	m = step(t, m, prsLoadedMsg{repo: repo, prs: nil})
	m = step(t, m, runsLoadedMsg{repo: repo, runs: nil})
	m = step(t, m, issuesLoadedMsg{repo: repo, issues: nil})

	// 60 alerts - far more than fit on screen.
	var alerts []gh.SecurityAlert
	for i := 0; i < 60; i++ {
		alerts = append(alerts, gh.SecurityAlert{
			Source: "dependabot", Severity: "medium",
			Detail:    "advisory number " + strconv.Itoa(i+1),
			Location:  "pkg-" + strconv.Itoa(i+1),
			HTMLURL:   "https://x/security/" + strconv.Itoa(i+1),
			CreatedAt: now,
		})
	}
	m = step(t, m, securityLoadedMsg{repo: repo, alerts: alerts})
	m = step(t, m, key("4"))

	if m.detail.secTable.Len() != 60 {
		t.Fatalf("expected all 60 alerts, got %d", m.detail.secTable.Len())
	}
	// Layout fits despite 60 rows (table scrolls).
	assertLayout(t, m.View(), w, h)

	// 'G' jumps to the last alert; the table scrolls to show it.
	m = step(t, m, key("G"))
	if m.detail.secTable.Cursor() != 59 {
		t.Fatalf("expected cursor at last row (59), got %d", m.detail.secTable.Cursor())
	}
	assertLayout(t, m.View(), w, h)
	if !strings.Contains(stripANSI(m.View()), "pkg-60") {
		t.Error("expected the last alert (pkg-60) visible after scrolling to end")
	}
}

func TestRepoVulnColumns(t *testing.T) {
	const w, h = 100, 24
	m := New(darkTheme)
	m = step(t, m, tea.WindowSizeMsg{Width: w, Height: h})
	m = step(t, m, userLoadedMsg{login: "octocat"})

	// Loading repos must NOT auto-scan vulnerabilities (cache-only now).
	nm, cmd := m.Update(reposLoadedMsg{repos: sampleRepos()})
	m = nm.(Model)
	if cmd != nil {
		t.Error("repos load should not trigger a vuln scan anymore")
	}
	if m.repos.vulnsLoaded {
		t.Error("no counts should be loaded before cache/scan")
	}

	// Cached counts apply instantly at startup.
	m = step(t, m, vulnsCacheLoadedMsg{counts: map[string]gh.VulnCounts{
		"acme/widget-api":     {Critical: 1, High: 11, Medium: 13, Low: 5, Total: 30, Known: true},
		"octocat/hello-world": {Known: true},
	}, savedAt: time.Now().Add(-time.Hour)})
	if !m.repos.vulnsLoaded || m.repos.vulns["acme/widget-api"].High != 11 {
		t.Fatal("cached vuln counts not applied")
	}
	plain := stripANSI(m.View())
	if !strings.Contains(plain, "Crit") || !strings.Contains(plain, "11") {
		t.Error("expected vuln columns with counts in the repo table")
	}
	assertLayout(t, m.View(), w, h)

	// 'v' triggers a live re-scan (sets scanning + returns a cmd).
	nm, cmd = m.Update(key("v"))
	m = nm.(Model)
	if !m.repos.vulnsScanning || cmd == nil {
		t.Fatal("'v' should start a live vuln scan")
	}
	if !strings.Contains(stripANSI(m.View()), "scanning") {
		t.Error("expected scanning indicator in footer")
	}

	// Fresh counts arrive, clearing the scanning state.
	m = step(t, m, vulnsLoadedMsg{counts: map[string]gh.VulnCounts{
		"acme/widget-api": {Critical: 2, High: 12, Known: true},
	}})
	if m.repos.vulnsScanning || m.repos.vulns["acme/widget-api"].Critical != 2 {
		t.Fatal("fresh counts should apply and clear scanning")
	}

	// ctrl+f (repo refresh) must NOT trigger a vuln scan.
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlF})
	m = nm.(Model)
	if m.repos.vulnsScanning {
		t.Error("ctrl+f should not start a vuln scan")
	}

	// Sorting by the Crit column works (s then 'c').
	m = step(t, m, key("s"))
	m = step(t, m, key("c"))
	if m.repos.table.Sorting() {
		t.Error("sort ribbon should close after choosing Crit")
	}
	assertLayout(t, m.View(), w, h)
}

func TestWorkflowDispatch(t *testing.T) {
	const w, h = 100, 26
	now := time.Now()
	m := New(darkTheme)
	m = step(t, m, tea.WindowSizeMsg{Width: w, Height: h})
	m = step(t, m, userLoadedMsg{login: "octocat"})
	m = step(t, m, reposLoadedMsg{repos: sampleRepos()})
	m = step(t, m, key("enter"))
	repo := m.detail.repo.NameWithOwner
	m = step(t, m, prsLoadedMsg{repo: repo, prs: nil})
	m = step(t, m, runsLoadedMsg{repo: repo, runs: []gh.Run{
		{DatabaseID: 1, WorkflowName: "CI", Status: "completed", Conclusion: "success", CreatedAt: now, URL: "https://x/1"},
	}})
	m = step(t, m, issuesLoadedMsg{repo: repo, issues: nil})
	m = step(t, m, securityLoadedMsg{repo: repo, unavailable: []string{"Dependabot", "code scanning", "secret scanning"}})
	m = step(t, m, key("2")) // workflows tab

	// 'r' opens the dispatch form and fires the info-load cmd.
	nm, cmd := m.Update(key("r"))
	m = nm.(Model)
	if !m.detail.dispatch.active || cmd == nil {
		t.Fatal("'r' should open the run-a-workflow form and load info")
	}
	// '?' must not open help while the form is up.
	if m.canOpenHelp() {
		t.Error("help should be blocked while the dispatch form is open")
	}

	// Workflows arrive (with a default branch).
	m = step(t, m, dispatchInfoLoadedMsg{repo: repo, defaultBranch: "main", workflows: []gh.Workflow{
		{ID: 10, Name: "CI", State: "active"},
		{ID: 20, Name: "Deploy", State: "active"},
	}})
	out := m.View()
	assertLayout(t, out, w, h)
	t.Log("\nDispatch (pick):\n" + stripANSI(out))

	// Move to "Deploy" and advance to the ref stage.
	m = step(t, m, key("j"))
	m = step(t, m, key("enter"))
	if m.detail.dispatch.stage != dispatchRef {
		t.Fatal("enter should advance to the ref stage")
	}
	if got := m.detail.dispatch.ref.Value(); got != "main" {
		t.Errorf("ref should default to the repo default branch, got %q", got)
	}
	out = m.View()
	assertLayout(t, out, w, h)
	t.Log("\nDispatch (ref):\n" + stripANSI(out))

	// enter triggers the dispatch (fires a gh cmd, sets working).
	nm, cmd = m.Update(key("enter"))
	m = nm.(Model)
	if cmd == nil || !m.detail.dispatch.working {
		t.Fatal("enter on the ref stage should dispatch the workflow")
	}

	// Success closes the form and flashes + reloads runs.
	nm, cmd = m.Update(dispatchDoneMsg{repo: repo, name: "Deploy", err: nil})
	m = nm.(Model)
	if m.detail.dispatch.active {
		t.Error("dispatch form should close on success")
	}
	if cmd == nil {
		t.Error("success should reload the runs list")
	}
	if !strings.Contains(m.detail.flash, "Deploy") {
		t.Errorf("expected a dispatched flash, got %q", m.detail.flash)
	}

	// Error keeps the form open with a message.
	_ = step(t, m, key("r"))
	m2 := step(t, m, key("r"))
	m2 = step(t, m2, dispatchInfoLoadedMsg{repo: repo, defaultBranch: "main", workflows: []gh.Workflow{{ID: 10, Name: "CI"}}})
	m2 = step(t, m2, key("enter")) // -> ref stage
	nm, _ = m2.Update(key("enter"))
	m2 = nm.(Model)
	m2 = step(t, m2, dispatchDoneMsg{repo: repo, name: "CI", err: errFake("no workflow_dispatch trigger")})
	if !m2.detail.dispatch.active || !m2.detail.dispatch.msgErr {
		t.Error("a dispatch error should keep the form open and show the error")
	}
}

type errFake string

func (e errFake) Error() string { return string(e) }
