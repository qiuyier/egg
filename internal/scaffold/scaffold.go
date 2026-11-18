package scaffold

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	templateModule = "github.com/qiuyier/egg"
	templateEnv    = "EGG"
)

var validName = regexp.MustCompile(`^[a-z][a-z0-9]*(-[a-z0-9]+)*$`)

// Generate creates an API project from the template and returns its absolute path.
// Pass checkout when generating from a source checkout to keep output outside it.
func Generate(source fs.FS, checkout, out, module, name string) (_ string, err error) {
	mod, err := fs.ReadFile(source, "go.mod")
	if err != nil {
		return "", fmt.Errorf("read template go.mod: %w", err)
	}
	if strings.SplitN(string(mod), "\n", 2)[0] != "module "+templateModule {
		return "", errors.New("run the generator from the egg directory")
	}

	out, err = outputPath(checkout, out)
	if err != nil {
		return "", err
	}
	if name == "" {
		name = filepath.Base(out)
	}
	if len(name) > 30 || !validName.MatchString(name) {
		return "", errors.New("project name must start with a lowercase letter and use at most 30 lowercase letters, digits or hyphens")
	}
	if module == "" {
		module = "example.com/" + name
	}
	if err := os.Mkdir(out, 0755); err != nil {
		return "", fmt.Errorf("create project directory: %w", err)
	}
	defer func() {
		if err != nil {
			err = errors.Join(err, os.RemoveAll(out))
		}
	}()

	if err := os.WriteFile(filepath.Join(out, "go.mod"), mod, 0644); err != nil {
		return "", err
	}
	edit := exec.Command("go", "mod", "edit", "-module="+module)
	edit.Dir = out
	edit.Env = append(os.Environ(), "GOWORK=off", "GOFLAGS=")
	if output, err := edit.CombinedOutput(); err != nil {
		return "", fmt.Errorf("set Go module: %w\n%s", err, strings.TrimSpace(string(output)))
	}

	files, goSource := projectReplacements(name, module)
	for _, file := range []string{".gitignore", "PROJECT.md", "compose.yaml", "configs/config.yaml", "go.sum"} {
		if err := copyFile(source, out, file, files[file]); err != nil {
			return "", err
		}
	}
	for _, folder := range []string{"cmd", "internal", "pkg"} {
		err := fs.WalkDir(source, folder, func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() && (strings.HasPrefix(entry.Name(), ".") || path == "cmd/egg" || path == "internal/scaffold") {
				return filepath.SkipDir
			}
			if !entry.Type().IsRegular() || strings.HasPrefix(entry.Name(), ".") || filepath.Ext(path) != ".go" {
				return nil
			}
			return copyFile(source, out, path, goSource)
		})
		if err != nil {
			return "", err
		}
	}
	return out, nil
}

// projectReplacements limits edits to the template's identity-bearing fields.
func projectReplacements(name, module string) (map[string]*strings.Replacer, *strings.Replacer) {
	namespace := strings.ReplaceAll(name, "-", "_")
	env := strings.ToUpper(namespace)
	files := map[string]*strings.Replacer{
		"PROJECT.md": strings.NewReplacer(
			"# egg API", "# "+name+" API",
			"EGG_", env+"_",
			"egg:egg@", namespace+":"+namespace+"@",
			"/egg?", "/"+namespace+"?",
		),
		"compose.yaml": strings.NewReplacer(
			"name: egg\n", "name: "+name+"\n",
			"EGG_", env+"_",
			": egg\n", ": "+namespace+"\n",
			`"-uegg"`, `"-u`+namespace+`"`,
			`"-pegg"`, `"-p`+namespace+`"`,
			`"egg"`, `"`+namespace+`"`,
		),
		"configs/config.yaml": strings.NewReplacer(
			"name: egg\n", "name: "+name+"\n",
			"EGG_", env+"_",
			`"egg:"`, `"`+name+`:"`,
		),
	}
	goSource := strings.NewReplacer(
		templateModule+"/", module+"/",
		templateEnv+"_", env+"_",
		`"`+templateEnv+`"`, `"`+env+`"`,
		`"egg"`, `"`+name+`"`,
		`"egg:`, `"`+name+":",
		"egg_http_", namespace+"_http_",
	)
	return files, goSource
}

func outputPath(checkout, out string) (string, error) {
	absolute, err := filepath.Abs(out)
	if err != nil {
		return "", err
	}
	parent, err := filepath.EvalSymlinks(filepath.Dir(absolute))
	if err != nil {
		return "", fmt.Errorf("resolve output parent: %w", err)
	}
	absolute = filepath.Join(parent, filepath.Base(absolute))
	if checkout == "" {
		return absolute, nil
	}
	root, err := filepath.EvalSymlinks(checkout)
	if err != nil {
		return "", err
	}
	relative, err := filepath.Rel(root, absolute)
	if err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", errors.New("output directory must be outside the template")
	}
	return absolute, nil
}

func copyFile(source fs.FS, out, file string, replacements *strings.Replacer) error {
	info, err := fs.Lstat(source, file)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("template file %q must be a regular file", file)
	}
	data, err := fs.ReadFile(source, file)
	if err != nil {
		return err
	}
	if replacements != nil {
		data = []byte(replacements.Replace(string(data)))
	}
	if file == "PROJECT.md" {
		file = "README.md"
	}
	target := filepath.Join(out, filepath.FromSlash(file))
	if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
		return err
	}
	return os.WriteFile(target, data, 0644)
}
