package features

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

type MatrixRow struct {
	ID         string `json:"id"`
	Competitor string `json:"competitor"`
	Feature    string `json:"feature"`
	Mapping    string `json:"mapping"`
}

type Document struct {
	Bullets []string    `json:"bullets"`
	Matrix  []MatrixRow `json:"matrix"`
}

func Parse(path string) (*Document, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open features file: %w", err)
	}
	defer f.Close()

	var (
		bullets []string
		matrix  []MatrixRow
		section string
	)

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if strings.HasPrefix(line, "### ") {
			switch {
			case strings.Contains(line, "Chef"):
				section = "Chef"
			case strings.Contains(line, "Ansible"):
				section = "Ansible"
			case strings.Contains(line, "Puppet"):
				section = "Puppet"
			case strings.Contains(line, "Salt"):
				section = "Salt"
			default:
				section = ""
			}
			continue
		}
		if strings.HasPrefix(line, "- ") {
			bullets = append(bullets, strings.TrimSpace(strings.TrimPrefix(line, "- ")))
			continue
		}
		if strings.HasPrefix(line, "| ") && section != "" {
			parts := strings.Split(line, "|")
			if len(parts) < 5 {
				continue
			}
			id := strings.TrimSpace(parts[1])
			feat := strings.TrimSpace(parts[2])
			mapping := strings.TrimSpace(parts[3])
			if id == "ID" || strings.HasPrefix(id, "---") || id == "" {
				continue
			}
			matrix = append(matrix, MatrixRow{
				ID:         id,
				Competitor: section,
				Feature:    feat,
				Mapping:    mapping,
			})
		}
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("scan features file: %w", err)
	}
	return &Document{
		Bullets: bullets,
		Matrix:  matrix,
	}, nil
}
