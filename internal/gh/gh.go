// Package gh wraps the GitHub `gh` CLI, shelling out and decoding JSON.
package gh

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Repo is a single repository from `gh repo list`.
type Repo struct {
	Name          string    `json:"name"`
	NameWithOwner string    `json:"nameWithOwner"`
	Description   string    `json:"description"`
	IsPrivate     bool      `json:"isPrivate"`
	PushedAt      time.Time `json:"pushedAt"`
	UpdatedAt     time.Time `json:"updatedAt"`
}

// user is a GitHub user reference (PR author / requested reviewer).
type user struct {
	Login string `json:"login"`
}

// reviewRequest is an entry in a PR's reviewRequests array. It may be a user
// (Login set) or a team (Name/Slug set), distinguished by __typename.
type reviewRequest struct {
	TypeName string `json:"__typename"`
	Login    string `json:"login"`
	Name     string `json:"name"`
	Slug     string `json:"slug"`
}

// PR is a single pull request from `gh pr list`.
type PR struct {
	Number         int             `json:"number"`
	Title          string          `json:"title"`
	Author         user            `json:"author"`
	ReviewRequests []reviewRequest `json:"reviewRequests"`
	HeadRefName    string          `json:"headRefName"`
	IsDraft        bool            `json:"isDraft"`
	UpdatedAt      time.Time       `json:"updatedAt"`
	URL            string          `json:"url"`
}

// ReviewerLogins returns the logins of users requested as reviewers.
func (p PR) ReviewerLogins() []string {
	var out []string
	for _, r := range p.ReviewRequests {
		if r.Login != "" {
			out = append(out, r.Login)
		}
	}
	return out
}

// AwaitsReviewFrom reports whether login is a requested reviewer on this PR.
func (p PR) AwaitsReviewFrom(login string) bool {
	for _, r := range p.ReviewRequests {
		if strings.EqualFold(r.Login, login) {
			return true
		}
	}
	return false
}

// Run is a single GitHub Actions workflow run from `gh run list`.
type Run struct {
	DatabaseID   int64     `json:"databaseId"`
	DisplayTitle string    `json:"displayTitle"`
	Status       string    `json:"status"`     // queued, in_progress, completed
	Conclusion   string    `json:"conclusion"` // success, failure, cancelled, ...
	WorkflowName string    `json:"workflowName"`
	HeadBranch   string    `json:"headBranch"`
	Event        string    `json:"event"`
	CreatedAt    time.Time `json:"createdAt"`
	URL          string    `json:"url"`
}

// Label is a PR/issue label.
type Label struct {
	Name string `json:"name"`
}

// Review is a submitted PR review.
type Review struct {
	Author            user      `json:"author"`
	State             string    `json:"state"` // APPROVED, CHANGES_REQUESTED, COMMENTED, ...
	Body              string    `json:"body"`
	AuthorAssociation string    `json:"authorAssociation"`
	SubmittedAt       time.Time `json:"submittedAt"`
}

// Check is one entry in a PR's status-check rollup. The rollup mixes GitHub
// Actions check runs (Name/Status/Conclusion) and legacy status contexts
// (Context/State), so both shapes are decoded here.
type Check struct {
	Name         string `json:"name"`
	WorkflowName string `json:"workflowName"`
	Status       string `json:"status"`
	Conclusion   string `json:"conclusion"`
	Context      string `json:"context"`
	State        string `json:"state"`
}

// Label/check display helpers.
func (c Check) DisplayName() string {
	if c.Name != "" {
		return c.Name
	}
	return c.Context
}

// Result returns a normalized lowercase outcome ("success", "failure",
// "pending", ...) across both check shapes.
func (c Check) Result() string {
	if c.Conclusion != "" {
		return strings.ToLower(c.Conclusion)
	}
	if c.State != "" {
		return strings.ToLower(c.State)
	}
	return strings.ToLower(c.Status)
}

// Comment is an issue-level comment on a pull request.
type Comment struct {
	Author            user      `json:"author"`
	Body              string    `json:"body"`
	CreatedAt         time.Time `json:"createdAt"`
	AuthorAssociation string    `json:"authorAssociation"`
}

// AuthorLogin exposes a comment author's login.
func (c Comment) AuthorLogin() string { return c.Author.Login }

// ChangedFile is one file in a PR's diff stat.
type ChangedFile struct {
	Path      string `json:"path"`
	Additions int    `json:"additions"`
	Deletions int    `json:"deletions"`
}

// PRDetail is the expanded view of a single pull request (`gh pr view`).
type PRDetail struct {
	Number           int           `json:"number"`
	Title            string        `json:"title"`
	State            string        `json:"state"` // OPEN, MERGED, CLOSED
	IsDraft          bool          `json:"isDraft"`
	Mergeable        string        `json:"mergeable"`        // MERGEABLE, CONFLICTING, UNKNOWN
	MergeStateStatus string        `json:"mergeStateStatus"` // CLEAN, BLOCKED, ...
	ReviewDecision   string        `json:"reviewDecision"`   // APPROVED, CHANGES_REQUESTED, REVIEW_REQUIRED, ""
	Additions        int           `json:"additions"`
	Deletions        int           `json:"deletions"`
	ChangedFiles     int           `json:"changedFiles"`
	BaseRefName      string        `json:"baseRefName"`
	HeadRefName      string        `json:"headRefName"`
	Author           user          `json:"author"`
	CreatedAt        time.Time     `json:"createdAt"`
	UpdatedAt        time.Time     `json:"updatedAt"`
	URL              string        `json:"url"`
	Body             string        `json:"body"`
	Labels           []Label       `json:"labels"`
	Reviews          []Review      `json:"reviews"`
	Comments         []Comment     `json:"comments"`
	Checks           []Check       `json:"statusCheckRollup"`
	Files            []ChangedFile `json:"files"`
}

// AuthorLogin exposes the PR author's login (Author's type is unexported).
func (p PRDetail) AuthorLogin() string { return p.Author.Login }

// Step is a single step within a job.
type Step struct {
	Name        string    `json:"name"`
	Number      int       `json:"number"`
	Status      string    `json:"status"`
	Conclusion  string    `json:"conclusion"`
	StartedAt   time.Time `json:"startedAt"`
	CompletedAt time.Time `json:"completedAt"`
}

// Job is a single job within a workflow run.
type Job struct {
	DatabaseID  int64     `json:"databaseId"`
	Name        string    `json:"name"`
	Status      string    `json:"status"`
	Conclusion  string    `json:"conclusion"`
	StartedAt   time.Time `json:"startedAt"`
	CompletedAt time.Time `json:"completedAt"`
	URL         string    `json:"url"`
	Steps       []Step    `json:"steps"`
}

// RunDetail is the expanded view of a single workflow run (`gh run view`).
type RunDetail struct {
	Number       int       `json:"number"`
	Attempt      int       `json:"attempt"`
	WorkflowName string    `json:"workflowName"`
	DisplayTitle string    `json:"displayTitle"`
	Status       string    `json:"status"`
	Conclusion   string    `json:"conclusion"`
	HeadBranch   string    `json:"headBranch"`
	HeadSha      string    `json:"headSha"`
	Event        string    `json:"event"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
	URL          string    `json:"url"`
	Jobs         []Job     `json:"jobs"`
}

// defaultTimeout bounds each gh invocation.
const defaultTimeout = 30 * time.Second

// runJSON executes `gh args...` and decodes stdout JSON into v.
func runJSON(ctx context.Context, v any, args ...string) error {
	out, err := run(ctx, args...)
	if err != nil {
		return err
	}
	if len(bytes.TrimSpace(out)) == 0 {
		return nil
	}
	if err := json.Unmarshal(out, v); err != nil {
		return fmt.Errorf("decoding `gh %s`: %w", strings.Join(args, " "), err)
	}
	return nil
}

// run executes `gh args...` and returns stdout, surfacing stderr on failure.
func run(ctx context.Context, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "gh", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return nil, fmt.Errorf("gh %s: %s", strings.Join(args, " "), msg)
	}
	return stdout.Bytes(), nil
}

// CurrentUser returns the authenticated account's login.
func CurrentUser(ctx context.Context) (string, error) {
	out, err := run(ctx, "api", "user", "--jq", ".login")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// apiRepo decodes the snake_case shape of GitHub's /user/repos endpoint.
type apiRepo struct {
	Name        string    `json:"name"`
	FullName    string    `json:"full_name"`
	Description string    `json:"description"`
	Private     bool      `json:"private"`
	PushedAt    time.Time `json:"pushed_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// ListRepos returns every repository the authenticated account can access -
// owned, organization, and collaborator repos, public and private - sorted by
// most recent push activity first.
//
// It uses the /user/repos API rather than `gh repo list` because the latter
// only returns repositories owned by the account.
func ListRepos(ctx context.Context) ([]Repo, error) {
	var apiRepos []apiRepo
	err := runJSON(ctx, &apiRepos,
		"api", "--paginate",
		"/user/repos?per_page=100&sort=pushed&affiliation=owner,collaborator,organization_member&visibility=all",
	)
	if err != nil {
		return nil, err
	}

	repos := make([]Repo, len(apiRepos))
	for i, r := range apiRepos {
		repos[i] = Repo{
			Name:          r.Name,
			NameWithOwner: r.FullName,
			Description:   r.Description,
			IsPrivate:     r.Private,
			PushedAt:      r.PushedAt,
			UpdatedAt:     r.UpdatedAt,
		}
	}
	sort.SliceStable(repos, func(i, j int) bool {
		return repos[i].activity().After(repos[j].activity())
	})
	return repos, nil
}

// activity is the timestamp used for ordering repos: the most recent of push
// and metadata update.
func (r Repo) activity() time.Time {
	if r.PushedAt.After(r.UpdatedAt) {
		return r.PushedAt
	}
	return r.UpdatedAt
}

// Activity exposes the ordering timestamp for display.
func (r Repo) Activity() time.Time { return r.activity() }

// ListPRs returns open pull requests for a repo, most recently updated first.
func ListPRs(ctx context.Context, nameWithOwner string) ([]PR, error) {
	var prs []PR
	err := runJSON(ctx, &prs,
		"pr", "list",
		"--repo", nameWithOwner,
		"--state", "open",
		"--limit", "100",
		"--json", "number,title,author,reviewRequests,headRefName,isDraft,updatedAt,url",
	)
	if err != nil {
		return nil, err
	}
	sort.SliceStable(prs, func(i, j int) bool {
		return prs[i].UpdatedAt.After(prs[j].UpdatedAt)
	})
	return prs, nil
}

// SearchRepo is the repository object returned by `gh search prs`.
type SearchRepo struct {
	Name          string `json:"name"`
	NameWithOwner string `json:"nameWithOwner"`
}

// SearchPR is a pull request returned by a cross-repo search.
type SearchPR struct {
	Number     int        `json:"number"`
	Title      string     `json:"title"`
	Repository SearchRepo `json:"repository"`
	Author     user       `json:"author"`
	URL        string     `json:"url"`
	IsDraft    bool       `json:"isDraft"`
	UpdatedAt  time.Time  `json:"updatedAt"`
}

// RepoName returns the owner/name of the PR's repository.
func (p SearchPR) RepoName() string {
	if p.Repository.NameWithOwner != "" {
		return p.Repository.NameWithOwner
	}
	return p.Repository.Name
}

// AuthorLogin exposes the PR author's login.
func (p SearchPR) AuthorLogin() string { return p.Author.Login }

// searchPRs runs `gh search prs <filter> --state=open`, newest first.
func searchPRs(ctx context.Context, filter string) ([]SearchPR, error) {
	var prs []SearchPR
	err := runJSON(ctx, &prs,
		"search", "prs", filter,
		"--state=open",
		"--limit", "100",
		"--json", "number,title,repository,author,url,updatedAt,isDraft",
	)
	if err != nil {
		return nil, err
	}
	sort.SliceStable(prs, func(i, j int) bool {
		return prs[i].UpdatedAt.After(prs[j].UpdatedAt)
	})
	return prs, nil
}

// SearchReviewRequestedPRs returns open PRs awaiting the current user's review.
func SearchReviewRequestedPRs(ctx context.Context) ([]SearchPR, error) {
	return searchPRs(ctx, "--review-requested=@me")
}

// SearchAuthoredPRs returns open PRs authored by the current user.
func SearchAuthoredPRs(ctx context.Context) ([]SearchPR, error) {
	return searchPRs(ctx, "--author=@me")
}

// VulnCounts is an open-Dependabot-alert breakdown for one repo. Known is false
// when the count couldn't be determined (no access).
type VulnCounts struct {
	Critical int
	High     int
	Medium   int
	Low      int
	Total    int
	Known    bool
}

// RepoVulnCounts returns open Dependabot alert counts per repo, fetched in
// batched GraphQL requests (so hundreds of repos cost only a handful of calls).
// Repos that error or can't be read are simply omitted from the map. The
// per-severity breakdown is taken from up to the first 100 alerts per repo;
// Total is exact.
func RepoVulnCounts(ctx context.Context, repos []Repo) (map[string]VulnCounts, error) {
	out := make(map[string]VulnCounts, len(repos))
	const batch = 40
	for start := 0; start < len(repos); start += batch {
		end := min(start+batch, len(repos))
		chunk := repos[start:end]

		var b strings.Builder
		b.WriteString("query {\n")
		for i, r := range chunk {
			owner, name, ok := strings.Cut(r.NameWithOwner, "/")
			if !ok {
				continue
			}
			fmt.Fprintf(&b, "a%d: repository(owner:%q, name:%q){ vulnerabilityAlerts(states:OPEN, first:100){ totalCount nodes{ securityVulnerability{ severity } } } }\n", i, owner, name)
		}
		b.WriteString("}\n")

		raw, err := run(ctx, "api", "graphql", "-f", "query="+b.String())
		if err != nil {
			continue // skip this batch; those repos stay unknown
		}
		var resp struct {
			Data map[string]*struct {
				VulnerabilityAlerts *struct {
					TotalCount int `json:"totalCount"`
					Nodes      []struct {
						SecurityVulnerability struct {
							Severity string `json:"severity"`
						} `json:"securityVulnerability"`
					} `json:"nodes"`
				} `json:"vulnerabilityAlerts"`
			} `json:"data"`
		}
		if err := json.Unmarshal(raw, &resp); err != nil {
			continue
		}
		for i, r := range chunk {
			node := resp.Data[fmt.Sprintf("a%d", i)]
			if node == nil || node.VulnerabilityAlerts == nil {
				continue
			}
			va := node.VulnerabilityAlerts
			vc := VulnCounts{Total: va.TotalCount, Known: true}
			for _, n := range va.Nodes {
				switch n.SecurityVulnerability.Severity {
				case "CRITICAL":
					vc.Critical++
				case "HIGH":
					vc.High++
				case "MODERATE":
					vc.Medium++
				case "LOW":
					vc.Low++
				}
			}
			out[r.NameWithOwner] = vc
		}
	}
	return out, nil
}

// ErrSecurityUnavailable means Dependabot alerts are disabled for the repo or
// the token lacks access (a 403/404), as opposed to a real failure.
var ErrSecurityUnavailable = errors.New("dependabot alerts unavailable")

// DependabotAlert is a single open vulnerability alert.
type DependabotAlert struct {
	Number     int       `json:"number"`
	State      string    `json:"state"`
	HTMLURL    string    `json:"html_url"`
	CreatedAt  time.Time `json:"created_at"`
	Dependency struct {
		Package struct {
			Ecosystem string `json:"ecosystem"`
			Name      string `json:"name"`
		} `json:"package"`
		ManifestPath string `json:"manifest_path"`
	} `json:"dependency"`
	SecurityAdvisory struct {
		GHSAID   string `json:"ghsa_id"`
		CVEID    string `json:"cve_id"`
		Severity string `json:"severity"`
		Summary  string `json:"summary"`
	} `json:"security_advisory"`
	SecurityVulnerability struct {
		VulnerableVersionRange string `json:"vulnerable_version_range"`
		FirstPatchedVersion    struct {
			Identifier string `json:"identifier"`
		} `json:"first_patched_version"`
	} `json:"security_vulnerability"`
}

func (a DependabotAlert) Severity() string    { return a.SecurityAdvisory.Severity }
func (a DependabotAlert) PackageName() string { return a.Dependency.Package.Name }
func (a DependabotAlert) Summary() string     { return a.SecurityAdvisory.Summary }
func (a DependabotAlert) PatchedVersion() string {
	return a.SecurityVulnerability.FirstPatchedVersion.Identifier
}

// SeverityRank orders severities high-to-low for sorting (critical highest).
func SeverityRank(severity string) int {
	switch strings.ToLower(severity) {
	case "critical":
		return 4
	case "high":
		return 3
	case "medium":
		return 2
	case "low":
		return 1
	default:
		return 0
	}
}

// ListDependabotAlerts returns ALL open Dependabot alerts for a repo, most
// severe first. It paginates through every page (gh merges the array pages), so
// repos with more than one page of alerts aren't truncated. Returns
// ErrSecurityUnavailable when alerts are disabled or the token lacks access.
func ListDependabotAlerts(ctx context.Context, nameWithOwner string) ([]DependabotAlert, error) {
	var alerts []DependabotAlert
	err := runJSON(ctx, &alerts, "api", "--paginate",
		fmt.Sprintf("repos/%s/dependabot/alerts?state=open&per_page=100", nameWithOwner))
	if err != nil {
		msg := strings.ToLower(err.Error())
		if strings.Contains(msg, "disabled") || strings.Contains(msg, "http 403") ||
			strings.Contains(msg, "http 404") || strings.Contains(msg, "must have") {
			return nil, ErrSecurityUnavailable
		}
		return nil, err
	}
	sort.SliceStable(alerts, func(i, j int) bool {
		return SeverityRank(alerts[i].Severity()) > SeverityRank(alerts[j].Severity())
	})
	return alerts, nil
}

// SecurityAlert is a normalized open security alert from any source
// (Dependabot, code scanning, or secret scanning).
type SecurityAlert struct {
	Source    string // "dependabot" | "code" | "secret"
	Severity  string // critical/high/medium/low (synthesized for some sources)
	Detail    string // advisory summary / rule description / secret type
	Location  string // package / path:line / (blank)
	HTMLURL   string
	CreatedAt time.Time
}

// listCodeScanningAlerts fetches open code-scanning alerts, normalized.
func listCodeScanningAlerts(ctx context.Context, nameWithOwner string) ([]SecurityAlert, error) {
	var raw []struct {
		HTMLURL   string    `json:"html_url"`
		CreatedAt time.Time `json:"created_at"`
		Rule      struct {
			Severity              string `json:"severity"`
			SecuritySeverityLevel string `json:"security_severity_level"`
			Description           string `json:"description"`
			Name                  string `json:"name"`
		} `json:"rule"`
		MostRecentInstance struct {
			Location struct {
				Path      string `json:"path"`
				StartLine int    `json:"start_line"`
			} `json:"location"`
		} `json:"most_recent_instance"`
	}
	err := runJSON(ctx, &raw, "api", "--paginate",
		fmt.Sprintf("repos/%s/code-scanning/alerts?state=open&per_page=100", nameWithOwner))
	if err != nil {
		return nil, err
	}
	out := make([]SecurityAlert, 0, len(raw))
	for _, a := range raw {
		detail := a.Rule.Description
		if detail == "" {
			detail = a.Rule.Name
		}
		loc := a.MostRecentInstance.Location.Path
		if loc != "" && a.MostRecentInstance.Location.StartLine > 0 {
			loc = fmt.Sprintf("%s:%d", loc, a.MostRecentInstance.Location.StartLine)
		}
		out = append(out, SecurityAlert{
			Source:    "code",
			Severity:  codeSeverity(a.Rule.SecuritySeverityLevel, a.Rule.Severity),
			Detail:    detail,
			Location:  loc,
			HTMLURL:   a.HTMLURL,
			CreatedAt: a.CreatedAt,
		})
	}
	return out, nil
}

// codeSeverity prefers the security severity level, falling back to mapping the
// generic rule severity (error/warning/note).
func codeSeverity(level, severity string) string {
	switch strings.ToLower(level) {
	case "critical", "high", "medium", "low":
		return strings.ToLower(level)
	}
	switch strings.ToLower(severity) {
	case "error":
		return "high"
	case "warning":
		return "medium"
	case "note":
		return "low"
	}
	return ""
}

// listSecretScanningAlerts fetches open secret-scanning alerts, normalized.
// Secrets have no severity, so they're synthesized as "high" to surface them.
func listSecretScanningAlerts(ctx context.Context, nameWithOwner string) ([]SecurityAlert, error) {
	var raw []struct {
		HTMLURL               string    `json:"html_url"`
		CreatedAt             time.Time `json:"created_at"`
		SecretType            string    `json:"secret_type"`
		SecretTypeDisplayName string    `json:"secret_type_display_name"`
	}
	err := runJSON(ctx, &raw, "api", "--paginate",
		fmt.Sprintf("repos/%s/secret-scanning/alerts?state=open&per_page=100", nameWithOwner))
	if err != nil {
		return nil, err
	}
	out := make([]SecurityAlert, 0, len(raw))
	for _, a := range raw {
		name := a.SecretTypeDisplayName
		if name == "" {
			name = a.SecretType
		}
		out = append(out, SecurityAlert{
			Source:    "secret",
			Severity:  "high",
			Detail:    name,
			HTMLURL:   a.HTMLURL,
			CreatedAt: a.CreatedAt,
		})
	}
	return out, nil
}

// RepoSecurityAlerts aggregates open alerts from Dependabot, code scanning, and
// secret scanning into one severity-sorted list. Sources that are disabled or
// inaccessible are reported in `unavailable` (by display name) rather than
// failing the whole view.
func RepoSecurityAlerts(ctx context.Context, nameWithOwner string) (alerts []SecurityAlert, unavailable []string) {
	// Dependabot.
	if dep, err := ListDependabotAlerts(ctx, nameWithOwner); err != nil {
		unavailable = append(unavailable, "Dependabot")
	} else {
		for _, a := range dep {
			alerts = append(alerts, SecurityAlert{
				Source: "dependabot", Severity: a.Severity(), Detail: a.Summary(),
				Location: a.PackageName(), HTMLURL: a.HTMLURL, CreatedAt: a.CreatedAt,
			})
		}
	}
	// Code scanning.
	if code, err := listCodeScanningAlerts(ctx, nameWithOwner); err != nil {
		unavailable = append(unavailable, "code scanning")
	} else {
		alerts = append(alerts, code...)
	}
	// Secret scanning.
	if sec, err := listSecretScanningAlerts(ctx, nameWithOwner); err != nil {
		unavailable = append(unavailable, "secret scanning")
	} else {
		alerts = append(alerts, sec...)
	}

	sort.SliceStable(alerts, func(i, j int) bool {
		return SeverityRank(alerts[i].Severity) > SeverityRank(alerts[j].Severity)
	})
	return alerts, unavailable
}

// Issue is a single issue from `gh issue list`.
type Issue struct {
	Number    int       `json:"number"`
	Title     string    `json:"title"`
	State     string    `json:"state"`
	Author    user      `json:"author"`
	Labels    []Label   `json:"labels"`
	UpdatedAt time.Time `json:"updatedAt"`
	URL       string    `json:"url"`
}

// AuthorLogin exposes the issue author's login.
func (i Issue) AuthorLogin() string { return i.Author.Login }

// IssueDetail is the expanded view of a single issue (`gh issue view`).
type IssueDetail struct {
	Number    int       `json:"number"`
	Title     string    `json:"title"`
	State     string    `json:"state"`
	Author    user      `json:"author"`
	Body      string    `json:"body"`
	Labels    []Label   `json:"labels"`
	Comments  []Comment `json:"comments"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
	URL       string    `json:"url"`
}

// AuthorLogin exposes the issue author's login.
func (d IssueDetail) AuthorLogin() string { return d.Author.Login }

// ListIssues returns open issues for a repo, most recently updated first.
func ListIssues(ctx context.Context, nameWithOwner string) ([]Issue, error) {
	var issues []Issue
	err := runJSON(ctx, &issues,
		"issue", "list",
		"--repo", nameWithOwner,
		"--state", "open",
		"--limit", "100",
		"--json", "number,title,state,author,labels,updatedAt,url",
	)
	if err != nil {
		return nil, err
	}
	sort.SliceStable(issues, func(i, j int) bool {
		return issues[i].UpdatedAt.After(issues[j].UpdatedAt)
	})
	return issues, nil
}

// GetIssue returns the expanded detail (body + comments) for a single issue.
func GetIssue(ctx context.Context, nameWithOwner string, number int) (IssueDetail, error) {
	var d IssueDetail
	err := runJSON(ctx, &d,
		"issue", "view", strconv.Itoa(number),
		"--repo", nameWithOwner,
		"--json", "number,title,state,author,body,labels,comments,createdAt,updatedAt,url",
	)
	if err != nil {
		return IssueDetail{}, err
	}
	return d, nil
}

// CommentIssue adds a comment to an issue.
func CommentIssue(ctx context.Context, nameWithOwner string, number int, body string) error {
	_, err := run(ctx, "issue", "comment", strconv.Itoa(number), "--repo", nameWithOwner, "--body", body)
	return err
}

// CloseIssue closes an issue.
func CloseIssue(ctx context.Context, nameWithOwner string, number int) error {
	_, err := run(ctx, "issue", "close", strconv.Itoa(number), "--repo", nameWithOwner)
	return err
}

// Notification is an item from the authenticated user's notification inbox.
type Notification struct {
	ID        string    `json:"id"`
	Unread    bool      `json:"unread"`
	Reason    string    `json:"reason"`
	UpdatedAt time.Time `json:"updated_at"`
	Subject   struct {
		Title string `json:"title"`
		Type  string `json:"type"` // PullRequest, Issue, CheckSuite, Release, ...
		URL   string `json:"url"`
	} `json:"subject"`
	Repository struct {
		FullName string `json:"full_name"`
	} `json:"repository"`
}

// RepoName returns the owner/name of the notification's repository.
func (n Notification) RepoName() string { return n.Repository.FullName }

// WebURL converts the subject's API URL to a github.com URL (falling back to
// the repository page).
func (n Notification) WebURL() string {
	u := n.Subject.URL
	if u == "" {
		return "https://github.com/" + n.Repository.FullName
	}
	w := strings.Replace(u, "https://api.github.com/repos/", "https://github.com/", 1)
	w = strings.Replace(w, "/pulls/", "/pull/", 1)
	return w
}

// PRNumber returns the pull request number when the subject is a PullRequest.
func (n Notification) PRNumber() (int, bool) {
	if n.Subject.Type != "PullRequest" {
		return 0, false
	}
	parts := strings.Split(n.Subject.URL, "/")
	num, err := strconv.Atoi(parts[len(parts)-1])
	if err != nil {
		return 0, false
	}
	return num, true
}

// ListNotificationsPage returns one page (up to 50) of the authenticated user's
// unread notifications, newest first. The API caps per_page at 50, so deeper
// backlogs are reached by requesting later pages rather than paginating the
// whole thing up front (which keeps the inbox fast).
func ListNotificationsPage(ctx context.Context, page int) ([]Notification, error) {
	if page < 1 {
		page = 1
	}
	var ns []Notification
	if err := runJSON(ctx, &ns, "api", fmt.Sprintf("notifications?per_page=50&page=%d", page)); err != nil {
		return nil, err
	}
	sort.SliceStable(ns, func(i, j int) bool {
		return ns[i].UpdatedAt.After(ns[j].UpdatedAt)
	})
	return ns, nil
}

// ListNotifications returns the first page of unread notifications.
func ListNotifications(ctx context.Context) ([]Notification, error) {
	return ListNotificationsPage(ctx, 1)
}

// MarkNotificationRead marks a single notification thread as read.
func MarkNotificationRead(ctx context.Context, id string) error {
	_, err := run(ctx, "api", "-X", "PATCH", "notifications/threads/"+id)
	return err
}

// MarkAllNotificationsRead marks every notification as read.
func MarkAllNotificationsRead(ctx context.Context) error {
	_, err := run(ctx, "api", "-X", "PUT", "notifications")
	return err
}

// ListRuns returns recent GitHub Actions runs for a repo, newest first.
func ListRuns(ctx context.Context, nameWithOwner string) ([]Run, error) {
	var runs []Run
	err := runJSON(ctx, &runs,
		"run", "list",
		"--repo", nameWithOwner,
		"--limit", "50",
		"--json", "databaseId,displayTitle,status,conclusion,workflowName,headBranch,event,createdAt,url",
	)
	if err != nil {
		return nil, err
	}
	sort.SliceStable(runs, func(i, j int) bool {
		return runs[i].CreatedAt.After(runs[j].CreatedAt)
	})
	return runs, nil
}

// GetRun returns the expanded detail (jobs + steps) for a single run.
func GetRun(ctx context.Context, nameWithOwner string, runID int64) (RunDetail, error) {
	var d RunDetail
	err := runJSON(ctx, &d,
		"run", "view", strconv.FormatInt(runID, 10),
		"--repo", nameWithOwner,
		"--json", "number,attempt,workflowName,displayTitle,status,conclusion,headBranch,headSha,event,createdAt,updatedAt,url,jobs",
	)
	if err != nil {
		return RunDetail{}, err
	}
	return d, nil
}

// GetRunLog returns logs for a run. When jobID > 0 it fetches that single job's
// log; otherwise it fetches the whole run's log. Logs are only available once a
// run has finished; gh returns an error while it is still in progress.
func GetRunLog(ctx context.Context, nameWithOwner string, runID, jobID int64) (string, error) {
	var args []string
	if jobID > 0 {
		args = []string{"run", "view", "--repo", nameWithOwner, "--job", strconv.FormatInt(jobID, 10), "--log"}
	} else {
		args = []string{"run", "view", strconv.FormatInt(runID, 10), "--repo", nameWithOwner, "--log"}
	}
	out, err := run(ctx, args...)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// RerunRun re-runs a workflow run. When failedOnly is true only failed jobs are
// re-run; otherwise the whole run is re-run.
func RerunRun(ctx context.Context, nameWithOwner string, runID int64, failedOnly bool) error {
	args := []string{"run", "rerun", strconv.FormatInt(runID, 10), "--repo", nameWithOwner}
	if failedOnly {
		args = append(args, "--failed")
	}
	_, err := run(ctx, args...)
	return err
}

// CancelRun cancels an in-progress workflow run.
func CancelRun(ctx context.Context, nameWithOwner string, runID int64) error {
	_, err := run(ctx, "run", "cancel", strconv.FormatInt(runID, 10), "--repo", nameWithOwner)
	return err
}

// AuthorLogin exposes a review author's login.
func (r Review) AuthorLogin() string { return r.Author.Login }

// GetPR returns the expanded detail for a single pull request.
func GetPR(ctx context.Context, nameWithOwner string, number int) (PRDetail, error) {
	var d PRDetail
	err := runJSON(ctx, &d,
		"pr", "view", strconv.Itoa(number),
		"--repo", nameWithOwner,
		"--json", "number,title,state,isDraft,mergeable,mergeStateStatus,reviewDecision,additions,deletions,changedFiles,baseRefName,headRefName,author,createdAt,updatedAt,url,body,labels,reviews,comments,statusCheckRollup,files",
	)
	if err != nil {
		return PRDetail{}, err
	}
	return d, nil
}

// GetPRDiff returns the unified diff for a pull request.
func GetPRDiff(ctx context.Context, nameWithOwner string, number int) (string, error) {
	out, err := run(ctx, "pr", "diff", strconv.Itoa(number), "--repo", nameWithOwner)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// ApprovePR submits an approving review on a pull request.
func ApprovePR(ctx context.Context, nameWithOwner string, number int) error {
	_, err := run(ctx, "pr", "review", strconv.Itoa(number), "--repo", nameWithOwner, "--approve")
	return err
}

// ReviewKind is a non-approving review type.
type ReviewKind string

const (
	ReviewRequestChanges ReviewKind = "request-changes"
	ReviewComment        ReviewKind = "comment"
)

// ReviewPR submits a request-changes or comment review with a (required) body.
func ReviewPR(ctx context.Context, nameWithOwner string, number int, kind ReviewKind, body string) error {
	_, err := run(ctx, "pr", "review", strconv.Itoa(number), "--repo", nameWithOwner, "--"+string(kind), "--body", body)
	return err
}

// MergeMethod selects how a PR is merged.
type MergeMethod string

const (
	MergeCommit MergeMethod = "merge"
	MergeSquash MergeMethod = "squash"
	MergeRebase MergeMethod = "rebase"
)

// MergePR merges a pull request using the given method.
func MergePR(ctx context.Context, nameWithOwner string, number int, method MergeMethod) error {
	_, err := run(ctx, "pr", "merge", strconv.Itoa(number), "--repo", nameWithOwner, "--"+string(method))
	return err
}

// ClosePR closes a pull request without merging it.
func ClosePR(ctx context.Context, nameWithOwner string, number int) error {
	_, err := run(ctx, "pr", "close", strconv.Itoa(number), "--repo", nameWithOwner)
	return err
}
