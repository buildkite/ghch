package ghch

import (
	"bufio"
	"bytes"
	"context"
	"log"
	"strings"
	"text/template"
	"time"

	"github.com/google/go-github/github"
)

// Changelog contains Sectionst
type Changelog struct {
	Sections []Section `json:"Sections"`
}

func insertNewChangelog(orig []byte, section string) string {
	var bf bytes.Buffer
	lineSnr := bufio.NewScanner(bytes.NewReader(orig))
	inserted := false
	for lineSnr.Scan() {
		line := lineSnr.Text()
		if !inserted && strings.HasPrefix(line, "## ") {
			bf.WriteString(section)
			bf.WriteString("\n\n")
			inserted = true
		}
		bf.WriteString(line)
		bf.WriteString("\n")
	}
	if !inserted {
		bf.WriteString(section)
	}
	return bf.String()
}

// Section contains changes between two revisions
type Section struct {
	PullRequests []*github.PullRequest `json:"pull_requests"`
	FromRevision string                `json:"from_revision"`
	ToRevision   string                `json:"to_revision"`
	ChangedAt    time.Time             `json:"changed_at"`
	Owner        string                `json:"owner"`
	Repo         string                `json:"repo"`
	HTMLURL      string                `json:"html_url"`

	Groups map[string][]*github.PullRequest
}

var (
	tmplStr = `{{$ret := . -}}
## [{{.ToRevision}}](https://github.com/{{.Owner}}/{{.Repo}}/tree/{{.ToRevision}}) ({{.ChangedAt.Format "2006-01-02"}})
[Full Changelog](https://github.com/{{.Owner}}/{{.Repo}}/compare/{{.FromRevision}}...{{.ToRevision}})

### Changed
{{- range .PullRequests}}
- {{.Title}} [#{{.Number}}](https://github.com/{{$ret.Owner}}/{{$ret.Repo}}/pull/{{.Number}}) (@{{.User.Login}})
{{- end}}`
	mdTmpl = &template.Template{}

	groupTmpl = `{{$ret := . -}}
## [{{.ToRevision}}](https://github.com/{{.Owner}}/{{.Repo}}/tree/{{.ToRevision}}) ({{.ChangedAt.Format "2006-01-02"}})
[Full Changelog](https://github.com/{{.Owner}}/{{.Repo}}/compare/{{.FromRevision}}...{{.ToRevision}})
{{range $group, $value := .Groups}}
### {{ $group | title }}
{{- range $value }}
- {{.Title}} [#{{.Number}}](https://github.com/{{$ret.Owner}}/{{$ret.Repo}}/pull/{{.Number}}) (@{{.User.Login}})
{{- end}}
{{end -}}`
	groupMdown = &template.Template{}
	groupFuncs = template.FuncMap{
		"title": strings.Title,
	}
)

func init() {
	var err error
	mdTmpl, err = template.New("md-changelog").Parse(tmplStr)
	if err != nil {
		log.Fatal(err)
	}
	groupMdown, err = template.New("md-changelog").Funcs(groupFuncs).Parse(groupTmpl)
	if err != nil {
		log.Fatal(err)
	}
}

func (rs Section) toMkdn() (string, error) {
	var b bytes.Buffer
	if len(rs.Groups) > 0 {
		err := groupMdown.Execute(&b, rs)
		return b.String(), err
	}

	err := mdTmpl.Execute(&b, rs)
	if err != nil {
		return "", err
	}
	return b.String(), nil
}

func (gh *Ghch) getSection(ctx context.Context, from, to string) (Section, error) {
	if from == "" {
		from, _ = gh.cmd("rev-list", "--max-parents=0", "HEAD")
		from = strings.TrimSpace(from)
		if len(from) > 12 {
			from = from[:12]
		}
	}
	r, err := gh.mergedPRs(ctx, from, to)
	if err != nil {
		return Section{}, err
	}
	t, err := gh.getChangedAt(to)
	if err != nil {
		return Section{}, err
	}
	owner, repo := gh.ownerAndRepo()
	htmlURL, err := gh.htmlURL(ctx, owner, repo)
	if err != nil {
		return Section{}, err
	}

	groups := make(map[string][]*github.PullRequest, 0)
	if gh.LabelHeadings {
		for _, pr := range r {
			group := "changed"
			if len(pr.Labels) > 0 {
				for _, l := range pr.Labels {
					if name, found := strings.CutPrefix(*l.Name, "release-heading/"); found {
						group = name
						break
					}
				}
			}

			if _, ok := groups[group]; !ok {
				groups[group] = make([]*github.PullRequest, 0)
			}
			groups[group] = append(groups[group], pr)
		}
	}

	return Section{
		PullRequests: r,
		FromRevision: from,
		ToRevision:   to,
		ChangedAt:    t,
		Owner:        owner,
		Repo:         repo,
		HTMLURL:      htmlURL,
		Groups:       groups,
	}, nil
}
