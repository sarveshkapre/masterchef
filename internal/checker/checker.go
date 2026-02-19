package checker

import (
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"

	"github.com/masterchef/masterchef/internal/planner"
)

type Item struct {
	ResourceID  string `json:"resource_id"`
	Type        string `json:"type"`
	Host        string `json:"host"`
	Simulatable bool   `json:"simulatable"`
	WouldChange bool   `json:"would_change"`
	Reason      string `json:"reason"`
	Diff        string `json:"diff,omitempty"`
}

type Coverage struct {
	Simulatable int `json:"simulatable"`
	Total       int `json:"total"`
}

type Report struct {
	TotalResources   int                 `json:"total_resources"`
	Simulatable      int                 `json:"simulatable"`
	NonSimulatable   int                 `json:"non_simulatable"`
	ChangesNeeded    int                 `json:"changes_needed"`
	Confidence       float64             `json:"confidence"`
	CoverageByType   map[string]Coverage `json:"coverage_by_type"`
	UnsupportedItems []Item              `json:"unsupported_items"`
	Items            []Item              `json:"items"`
}

func Run(p *planner.Plan) Report {
	rep := Report{
		CoverageByType: map[string]Coverage{},
		Items:          make([]Item, 0, len(p.Steps)),
	}
	for _, step := range p.Steps {
		r := step.Resource
		it := Item{
			ResourceID: r.ID,
			Type:       r.Type,
			Host:       r.Host,
		}
		rep.TotalResources++
		cov := rep.CoverageByType[r.Type]
		cov.Total++

		switch step.Host.Transport {
		case "local", "winrm":
			switch r.Type {
			case "file":
				it.Simulatable = true
				cov.Simulatable++
				current, err := os.ReadFile(r.Path)
				if err == nil && string(current) == r.Content {
					it.WouldChange = false
					it.Reason = "file already in desired state"
				} else {
					it.WouldChange = true
					it.Reason = "file content differs or does not exist"
					oldContent := ""
					if err == nil {
						oldContent = string(current)
					}
					it.Diff = unifiedDiff(r.Path, oldContent, r.Content)
				}
			case "command":
				it.Simulatable = true
				cov.Simulatable++
				if r.Creates != "" {
					if _, err := os.Stat(r.Creates); err == nil {
						it.WouldChange = false
						it.Reason = "creates path already exists"
						break
					}
				}
				if r.OnlyIf != "" {
					if err := exec.Command("sh", "-c", r.OnlyIf).Run(); err != nil {
						it.WouldChange = false
						it.Reason = "only_if condition failed"
						break
					}
				}
				if r.Unless != "" {
					if err := exec.Command("sh", "-c", r.Unless).Run(); err == nil {
						it.WouldChange = false
						it.Reason = "unless condition succeeded"
						break
					}
				}
				it.WouldChange = true
				it.Reason = "command would execute"
			default:
				it.Simulatable = false
				it.Reason = "unsupported resource type for simulation"
			}
		default:
			it.Simulatable = false
			it.Reason = "unsupported transport for simulation"
		}

		rep.CoverageByType[r.Type] = cov
		if it.Simulatable {
			rep.Simulatable++
		} else {
			rep.NonSimulatable++
			rep.UnsupportedItems = append(rep.UnsupportedItems, it)
		}
		if it.WouldChange {
			rep.ChangesNeeded++
		}
		rep.Items = append(rep.Items, it)
	}

	if rep.TotalResources > 0 {
		rep.Confidence = float64(rep.Simulatable) / float64(rep.TotalResources)
	}
	sort.Slice(rep.Items, func(i, j int) bool {
		return rep.Items[i].ResourceID < rep.Items[j].ResourceID
	})
	return rep
}

func unifiedDiff(path, oldContent, newContent string) string {
	oldLines := splitLines(oldContent)
	newLines := splitLines(newContent)
	var b strings.Builder
	b.WriteString(fmt.Sprintf("--- %s\n", path))
	b.WriteString(fmt.Sprintf("+++ %s\n", path))
	max := len(oldLines)
	if len(newLines) > max {
		max = len(newLines)
	}
	for i := 0; i < max; i++ {
		var oldLine, newLine string
		if i < len(oldLines) {
			oldLine = oldLines[i]
		}
		if i < len(newLines) {
			newLine = newLines[i]
		}
		if oldLine == newLine {
			if oldLine != "" {
				b.WriteString(" " + oldLine + "\n")
			}
			continue
		}
		if oldLine != "" {
			b.WriteString("-" + oldLine + "\n")
		}
		if newLine != "" {
			b.WriteString("+" + newLine + "\n")
		}
	}
	return b.String()
}

func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	raw := strings.Split(strings.ReplaceAll(s, "\r\n", "\n"), "\n")
	if len(raw) > 0 && raw[len(raw)-1] == "" {
		raw = raw[:len(raw)-1]
	}
	return raw
}
