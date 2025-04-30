package cli

import (
	"fmt"
	"io/fs"
	"os"
	"slices"
	"strings"

	"github.com/goccy/go-yaml"
	"github.com/goccy/go-yaml/ast"
	"github.com/goccy/go-yaml/parser"
	"github.com/rwx-research/mint-cli/internal/errors"
)

type YAMLDoc struct {
	astFile  *ast.File
	original string
	latest   *string
}

func ParseYAMLDoc(content string) (*YAMLDoc, error) {
	astFile, err := parser.ParseBytes([]byte(content), parser.ParseComments)
	if err != nil {
		return nil, err
	}
	latest := astFile.String()

	return &YAMLDoc{astFile: astFile, original: latest, latest: &latest}, nil
}

func ParseYAMLFile(path string) (*YAMLDoc, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	return ParseYAMLDoc(string(content))
}

func (doc *YAMLDoc) Bytes() []byte {
	return []byte(doc.String())
}

func (doc *YAMLDoc) String() string {
	if doc.latest == nil {
		s := doc.astFile.String()
		doc.latest = &s
	}
	return *doc.latest
}

func (doc *YAMLDoc) HasChanges() bool {
	return doc.original != doc.String()
}

func (doc *YAMLDoc) HasBase() bool {
	return doc.hasPath("$.base")
}

func (doc *YAMLDoc) HasTasks() bool {
	return doc.hasPath("$.tasks")
}

func (doc *YAMLDoc) ReadStringAtPath(yamlPath string) (string, error) {
	node, err := doc.getNodeAtPath(yamlPath)
	if err != nil {
		return "", err
	}

	return node.String(), nil
}

func (doc *YAMLDoc) TryReadStringAtPath(yamlPath string) string {
	str, err := doc.ReadStringAtPath(yamlPath)
	if err != nil {
		return ""
	}
	return str
}

func (doc *YAMLDoc) InsertBefore(beforeYamlPath string, value any) error {
	if strings.Count(beforeYamlPath, ".") != 1 {
		return errors.New("must provide a root yaml field in the form of \"$.fieldname\"")
	}

	p, err := yaml.PathString(beforeYamlPath)
	if err != nil {
		panic(err)
	}

	// We can't use doc.astFile because it may have already been modified and
	// we need the original index for the relative yaml node.
	reparsedFile, err := parser.ParseBytes([]byte(doc.astFile.String()), parser.ParseComments)
	if err != nil {
		return err
	}

	relativeNode, err := p.FilterFile(reparsedFile)
	if err != nil {
		return err
	}

	// token: value for the given beforeYamlPath
	// token.Prev: the separator token, eg. ":"
	// token.Prev.Prev: key for the given beforeYamlPath
	token := relativeNode.GetToken()
	idx := token.Prev.Prev.Position.Offset - 1

	node, err := yaml.NewEncoder(nil).EncodeToNode(value)
	if err != nil {
		return err
	}

	toInsert := fmt.Appendf([]byte(node.String()), "\n\n")
	result := slices.Insert([]byte(doc.astFile.String()), idx, toInsert...)

	err = doc.reparseAst(string(result))
	if err != nil {
		return err
	}

	return nil
}

func (doc *YAMLDoc) MergeAtPath(yamlPath string, value any) error {
	p, err := yaml.PathString(yamlPath)
	if err != nil {
		panic(err)
	}

	node, err := yaml.NewEncoder(nil).EncodeToNode(value)
	if err != nil {
		return err
	}

	err = p.MergeFromNode(doc.astFile, node)
	if err != nil {
		return err
	}

	doc.modified()
	return nil
}

func (doc *YAMLDoc) ReplaceAtPath(yamlPath string, replacement any) error {
	p, err := yaml.PathString(yamlPath)
	if err != nil {
		panic(err)
	}

	// Ensure the path exists
	if _, err := p.FilterFile(doc.astFile); err != nil {
		return err
	}

	node, err := yaml.NewEncoder(nil).EncodeToNode(replacement)
	if err != nil {
		return err
	}

	err = p.ReplaceWithNode(doc.astFile, node)
	if err != nil {
		return err
	}

	doc.modified()
	return nil
}

func (doc *YAMLDoc) SetAtPath(yamlPath string, value any) error {
	pathParts := strings.Split(yamlPath, ".")
	field := pathParts[len(pathParts)-1]

	parent := strings.Join(pathParts[0:len(pathParts)-1], ".")
	path, err := yaml.PathString(parent)
	if err != nil {
		panic(err)
	}

	node, err := yaml.NewEncoder(nil).EncodeToNode(map[string]any{
		field: value,
	})
	if err != nil {
		return err
	}

	err = path.MergeFromNode(doc.astFile, node)
	if err != nil {
		return err
	}

	doc.modified()
	return nil
}

func (doc *YAMLDoc) ForEachNode(yamlPath string, f func(node ast.Node) error) error {
	node, err := doc.getNodeAtPath(yamlPath)
	if err != nil {
		return err
	}

	seqNode, ok := node.(*ast.SequenceNode)
	if !ok {
		return fmt.Errorf("expected sequence node, got %T", node)
	}

	for _, valueNode := range seqNode.Values {
		if valueNode == nil {
			continue
		}
		if err := f(valueNode); err != nil {
			return err
		}
	}
	return nil
}

func (doc *YAMLDoc) WriteFile(path string) error {
	// Inherit permissions from the existing file if it exists
	mode := fs.FileMode(0644)
	if fi, err := os.Stat(path); err == nil {
		mode = fi.Mode()
	}

	return os.WriteFile(path, doc.Bytes(), mode)
}

func (doc *YAMLDoc) getNodeAtPath(yamlPath string) (ast.Node, error) {
	p, err := yaml.PathString(yamlPath)
	if err != nil {
		panic(err)
	}

	return p.FilterFile(doc.astFile)
}

func (doc *YAMLDoc) hasPath(yamlPath string) bool {
	_, err := doc.getNodeAtPath(yamlPath)
	return err == nil
}

func (doc *YAMLDoc) modified() {
	doc.latest = nil
}

func (doc *YAMLDoc) reparseAst(contents string) error {
	astFile, err := parser.ParseBytes([]byte(contents), parser.ParseComments)
	if err != nil {
		return err
	}

	doc.astFile = astFile
	doc.latest = nil
	return nil
}
