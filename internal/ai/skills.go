package ai

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type SkillInfo struct {
	Name   string `json:"name"`
	Files  int    `json:"files"`
	Source string `json:"source"`
}

var builtinSkillAliases = map[string]string{
	"gongkao-huasheng13": "ref-skills/huasheng13-skill-master",
	"shenlun-master":     "ref-skills/Shenlun.skill-master",
}

// ResolveSkill loads a local skill folder (SKILL.md plus references/*.md).
// User-imported skills are resolved by the API layer first and take priority.
func (m *Manager) ResolveSkill(name string) (string, *SkillInfo) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", nil
	}
	root := filepath.Dir(m.webDir)
	candidates := []string{
		filepath.Join(root, ".reasonix", "skills", name),
		filepath.Join(root, "ref-skills", name),
	}
	if alias := builtinSkillAliases[name]; alias != "" {
		candidates = append(candidates, filepath.Join(root, alias))
	}
	for _, dir := range candidates {
		text, files, ok := readSkillDir(dir)
		if ok {
			return text, &SkillInfo{Name: name, Files: files, Source: "builtin"}
		}
	}
	return "", nil
}

func readSkillDir(dir string) (string, int, bool) {
	mainPath := filepath.Join(dir, "SKILL.md")
	main, err := os.ReadFile(mainPath)
	if err != nil {
		return "", 0, false
	}
	parts := []string{"===== Skill: " + filepath.Base(dir) + "（自动注入） =====", "[SKILL.md]\n" + string(main)}
	files := 1
	refsDir := filepath.Join(dir, "references")
	var refs []string
	_ = filepath.WalkDir(refsDir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(strings.ToLower(path), ".md") {
			return nil
		}
		refs = append(refs, path)
		return nil
	})
	sort.Strings(refs)
	for _, path := range refs {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		rel, _ := filepath.Rel(dir, path)
		parts = append(parts, "["+rel+"]\n"+string(data))
		files++
	}
	parts = append(parts, "===== Skill 结束 =====")
	text := strings.Join(parts, "\n\n")
	const maxSkillBytes = 2 << 20
	if len(text) > maxSkillBytes {
		text = text[:maxSkillBytes]
	}
	return text, files, true
}
