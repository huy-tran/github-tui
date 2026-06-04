package ui

import (
	"context"
	"os/exec"
	"runtime"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/huy-tran/github-tui/internal/cache"
	"github.com/huy-tran/github-tui/internal/gh"
)

// autoRefreshInterval is how often live screens silently re-poll while a
// workflow run is active.
const autoRefreshInterval = 5 * time.Second

// autoRefreshTickMsg fires on the recurring auto-refresh timer.
type autoRefreshTickMsg struct{}

// autoRefreshTickCmd schedules the next auto-refresh tick.
func autoRefreshTickCmd() tea.Cmd {
	return tea.Tick(autoRefreshInterval, func(time.Time) tea.Msg { return autoRefreshTickMsg{} })
}

// --- Messages -------------------------------------------------------------

type reposLoadedMsg struct{ repos []gh.Repo }

// vulnsLoadedMsg carries freshly-scanned per-repo Dependabot alert counts.
type vulnsLoadedMsg struct{ counts map[string]gh.VulnCounts }

// vulnsCacheLoadedMsg carries cached alert counts shown instantly at startup.
type vulnsCacheLoadedMsg struct {
	counts  map[string]gh.VulnCounts
	savedAt time.Time
}

// reposCacheLoadedMsg carries repositories read from the on-disk cache (shown
// instantly at startup while the network fetch is in flight).
type reposCacheLoadedMsg struct {
	repos   []gh.Repo
	savedAt time.Time
}
type prsLoadedMsg struct {
	repo string
	prs  []gh.PR
}
type runsLoadedMsg struct {
	repo string
	runs []gh.Run
}
type userLoadedMsg struct{ login string }
type myPRsLoadedMsg struct {
	review   []gh.SearchPR
	authored []gh.SearchPR
}
type runDetailLoadedMsg struct {
	repo   string
	runID  int64
	detail gh.RunDetail
}

// openRunMsg asks the root model to drill into a workflow run's detail screen.
type openRunMsg struct {
	repo string
	run  gh.Run
}

type runLogLoadedMsg struct {
	repo  string
	runID int64
	jobID int64
	log   string
}

// runActionDoneMsg reports the result of a rerun/cancel action.
type runActionDoneMsg struct {
	repo   string
	runID  int64
	action string // "rerun" | "cancel"
	err    error
}

type notifsLoadedMsg struct {
	notifs []gh.Notification
	page   int
}

// notifActionDoneMsg reports the result of a mark-as-read action.
type notifActionDoneMsg struct {
	id  string
	err error
}

// notifsMarkedAllMsg reports the result of mark-all-read.
type notifsMarkedAllMsg struct{ err error }

// openPRMsg asks the root model to drill into a pull request's detail screen.
type openPRMsg struct {
	repo string
	pr   gh.PR
}

// openIssueMsg asks the root model to drill into an issue's detail screen.
type openIssueMsg struct {
	repo  string
	issue gh.Issue
}

type issuesLoadedMsg struct {
	repo   string
	issues []gh.Issue
}

type issueDetailLoadedMsg struct {
	repo   string
	number int
	detail gh.IssueDetail
}

// issueActionDoneMsg reports the result of a comment/close action.
type issueActionDoneMsg struct {
	repo   string
	number int
	action string // "comment" | "close"
	err    error
}

type prDetailLoadedMsg struct {
	repo   string
	number int
	detail gh.PRDetail
}

type prDiffLoadedMsg struct {
	repo   string
	number int
	diff   string
}

// prActionDoneMsg reports the result of an approve/merge action.
type prActionDoneMsg struct {
	repo   string
	number int
	action string // "approve" | "merge"
	err    error
}

// mentionUsersLoadedMsg carries the repo's @-mentionable users for the review
// composer's autocomplete.
type mentionUsersLoadedMsg struct {
	repo   string
	logins []string
}

// errMsg carries an error and the logical context it occurred in.
type errMsg struct {
	context string
	err     error
}

func (e errMsg) Error() string { return e.err.Error() }

// --- Commands -------------------------------------------------------------

func loadUserCmd() tea.Cmd {
	return func() tea.Msg {
		login, err := gh.CurrentUser(context.Background())
		if err != nil {
			return errMsg{context: "loading account", err: err}
		}
		return userLoadedMsg{login: login}
	}
}

func loadMyPRsCmd() tea.Cmd {
	return func() tea.Msg {
		review, err := gh.SearchReviewRequestedPRs(context.Background())
		if err != nil {
			return errMsg{context: "loading my PRs", err: err}
		}
		authored, err := gh.SearchAuthoredPRs(context.Background())
		if err != nil {
			return errMsg{context: "loading my PRs", err: err}
		}
		return myPRsLoadedMsg{review: review, authored: authored}
	}
}

func loadNotifsCmd() tea.Cmd { return loadNotifsPageCmd(1) }

func loadNotifsPageCmd(page int) tea.Cmd {
	return func() tea.Msg {
		ns, err := gh.ListNotificationsPage(context.Background(), page)
		if err != nil {
			return errMsg{context: "loading notifications", err: err}
		}
		return notifsLoadedMsg{notifs: ns, page: page}
	}
}

func markNotifReadCmd(id string) tea.Cmd {
	return func() tea.Msg {
		err := gh.MarkNotificationRead(context.Background(), id)
		return notifActionDoneMsg{id: id, err: err}
	}
}

func markAllNotifsReadCmd() tea.Cmd {
	return func() tea.Msg {
		return notifsMarkedAllMsg{err: gh.MarkAllNotificationsRead(context.Background())}
	}
}

func loadReposCmd() tea.Cmd {
	return func() tea.Msg {
		repos, err := gh.ListRepos(context.Background())
		if err != nil {
			return errMsg{context: "loading repositories", err: err}
		}
		_ = cache.WriteRepos(repos) // best-effort write-through
		return reposLoadedMsg{repos: repos}
	}
}

// loadVulnsCmd scans per-repo vulnerability counts live and writes them through
// to the on-disk cache.
func loadVulnsCmd(repos []gh.Repo) tea.Cmd {
	return func() tea.Msg {
		counts, err := gh.RepoVulnCounts(context.Background(), repos)
		if err != nil {
			return vulnsLoadedMsg{counts: map[string]gh.VulnCounts{}}
		}
		_ = cache.WriteVulns(counts)
		return vulnsLoadedMsg{counts: counts}
	}
}

// loadVulnsCacheCmd reads cached alert counts from disk (a miss yields empties).
func loadVulnsCacheCmd() tea.Cmd {
	return func() tea.Msg {
		counts, savedAt, err := cache.ReadVulns()
		if err != nil {
			return vulnsCacheLoadedMsg{}
		}
		return vulnsCacheLoadedMsg{counts: counts, savedAt: savedAt}
	}
}

// loadReposCacheCmd reads the cached repository list from disk. A cache miss
// yields an empty message, which the model ignores.
func loadReposCacheCmd() tea.Cmd {
	return func() tea.Msg {
		repos, savedAt, err := cache.ReadRepos()
		if err != nil {
			return reposCacheLoadedMsg{}
		}
		return reposCacheLoadedMsg{repos: repos, savedAt: savedAt}
	}
}

func loadPRsCmd(repo string) tea.Cmd {
	return func() tea.Msg {
		prs, err := gh.ListPRs(context.Background(), repo)
		if err != nil {
			return errMsg{context: "loading pull requests", err: err}
		}
		return prsLoadedMsg{repo: repo, prs: prs}
	}
}

type dispatchInfoLoadedMsg struct {
	repo          string
	workflows     []gh.Workflow
	defaultBranch string
	err           error
}

// dispatchDoneMsg reports the result of triggering a workflow.
type dispatchDoneMsg struct {
	repo string
	name string
	err  error
}

// loadDispatchInfoCmd fetches the repo's workflows and default branch for the
// "run a workflow" form.
func loadDispatchInfoCmd(repo string) tea.Cmd {
	return func() tea.Msg {
		wfs, err := gh.ListWorkflows(context.Background(), repo)
		if err != nil {
			return dispatchInfoLoadedMsg{repo: repo, err: err}
		}
		branch, _ := gh.DefaultBranch(context.Background(), repo) // best effort
		return dispatchInfoLoadedMsg{repo: repo, workflows: wfs, defaultBranch: branch}
	}
}

func dispatchWorkflowCmd(repo string, id int64, name, ref string) tea.Cmd {
	return func() tea.Msg {
		err := gh.DispatchWorkflow(context.Background(), repo, id, ref)
		return dispatchDoneMsg{repo: repo, name: name, err: err}
	}
}

func loadIssuesCmd(repo string) tea.Cmd {
	return func() tea.Msg {
		issues, err := gh.ListIssues(context.Background(), repo)
		if err != nil {
			return errMsg{context: "loading issues", err: err}
		}
		return issuesLoadedMsg{repo: repo, issues: issues}
	}
}

func loadIssueDetailCmd(repo string, number int) tea.Cmd {
	return func() tea.Msg {
		d, err := gh.GetIssue(context.Background(), repo, number)
		if err != nil {
			return errMsg{context: "loading issue", err: err}
		}
		return issueDetailLoadedMsg{repo: repo, number: number, detail: d}
	}
}

func commentIssueCmd(repo string, number int, body string) tea.Cmd {
	return func() tea.Msg {
		err := gh.CommentIssue(context.Background(), repo, number, body)
		return issueActionDoneMsg{repo: repo, number: number, action: "comment", err: err}
	}
}

func closeIssueCmd(repo string, number int) tea.Cmd {
	return func() tea.Msg {
		err := gh.CloseIssue(context.Background(), repo, number)
		return issueActionDoneMsg{repo: repo, number: number, action: "close", err: err}
	}
}

// securityLoadedMsg carries the unified security alerts (Dependabot + code +
// secret scanning) for a repo, plus the sources that were unavailable.
type securityLoadedMsg struct {
	repo        string
	alerts      []gh.SecurityAlert
	unavailable []string
}

func loadSecurityCmd(repo string) tea.Cmd {
	return func() tea.Msg {
		alerts, unavailable := gh.RepoSecurityAlerts(context.Background(), repo)
		return securityLoadedMsg{repo: repo, alerts: alerts, unavailable: unavailable}
	}
}

func loadRunsCmd(repo string) tea.Cmd {
	return func() tea.Msg {
		runs, err := gh.ListRuns(context.Background(), repo)
		if err != nil {
			return errMsg{context: "loading workflow runs", err: err}
		}
		return runsLoadedMsg{repo: repo, runs: runs}
	}
}

func loadRunDetailCmd(repo string, runID int64) tea.Cmd {
	return func() tea.Msg {
		d, err := gh.GetRun(context.Background(), repo, runID)
		if err != nil {
			return errMsg{context: "loading workflow run", err: err}
		}
		return runDetailLoadedMsg{repo: repo, runID: runID, detail: d}
	}
}

func loadPRDetailCmd(repo string, number int) tea.Cmd {
	return func() tea.Msg {
		d, err := gh.GetPR(context.Background(), repo, number)
		if err != nil {
			return errMsg{context: "loading pull request", err: err}
		}
		return prDetailLoadedMsg{repo: repo, number: number, detail: d}
	}
}

func loadPRDiffCmd(repo string, number int) tea.Cmd {
	return func() tea.Msg {
		diff, err := gh.GetPRDiff(context.Background(), repo, number)
		if err != nil {
			return errMsg{context: "loading diff", err: err}
		}
		return prDiffLoadedMsg{repo: repo, number: number, diff: diff}
	}
}

func approvePRCmd(repo string, number int) tea.Cmd {
	return func() tea.Msg {
		err := gh.ApprovePR(context.Background(), repo, number)
		return prActionDoneMsg{repo: repo, number: number, action: "approve", err: err}
	}
}

func mergePRCmd(repo string, number int, method gh.MergeMethod) tea.Cmd {
	return func() tea.Msg {
		err := gh.MergePR(context.Background(), repo, number, method)
		return prActionDoneMsg{repo: repo, number: number, action: "merge", err: err}
	}
}

func closePRCmd(repo string, number int) tea.Cmd {
	return func() tea.Msg {
		err := gh.ClosePR(context.Background(), repo, number)
		return prActionDoneMsg{repo: repo, number: number, action: "close", err: err}
	}
}

// loadMentionUsersCmd fetches the repo's @-mentionable users for the composer.
// A failure is non-fatal: the picker simply stays empty.
func loadMentionUsersCmd(repo string) tea.Cmd {
	return func() tea.Msg {
		logins, err := gh.MentionableUsers(context.Background(), repo)
		if err != nil {
			return mentionUsersLoadedMsg{repo: repo}
		}
		return mentionUsersLoadedMsg{repo: repo, logins: logins}
	}
}

func reviewPRCmd(repo string, number int, kind gh.ReviewKind, body string) tea.Cmd {
	return func() tea.Msg {
		err := gh.ReviewPR(context.Background(), repo, number, kind, body)
		return prActionDoneMsg{repo: repo, number: number, action: string(kind), err: err}
	}
}

func loadRunLogCmd(repo string, runID, jobID int64) tea.Cmd {
	return func() tea.Msg {
		log, err := gh.GetRunLog(context.Background(), repo, runID, jobID)
		if err != nil {
			return errMsg{context: "loading logs", err: err}
		}
		return runLogLoadedMsg{repo: repo, runID: runID, jobID: jobID, log: log}
	}
}

func rerunRunCmd(repo string, runID int64, failedOnly bool) tea.Cmd {
	return func() tea.Msg {
		err := gh.RerunRun(context.Background(), repo, runID, failedOnly)
		return runActionDoneMsg{repo: repo, runID: runID, action: "rerun", err: err}
	}
}

func cancelRunCmd(repo string, runID int64) tea.Cmd {
	return func() tea.Msg {
		err := gh.CancelRun(context.Background(), repo, runID)
		return runActionDoneMsg{repo: repo, runID: runID, action: "cancel", err: err}
	}
}

// openURLCmd opens a URL in the default browser; errors are surfaced quietly.
func openURLCmd(url string) tea.Cmd {
	return func() tea.Msg {
		var cmd *exec.Cmd
		switch runtime.GOOS {
		case "windows":
			cmd = exec.Command("cmd", "/c", "start", "", url)
		case "darwin":
			cmd = exec.Command("open", url)
		default:
			cmd = exec.Command("xdg-open", url)
		}
		if err := cmd.Start(); err != nil {
			return errMsg{context: "opening browser", err: err}
		}
		return nil
	}
}
