package cli

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/goccy/go-yaml"
	"github.com/goccy/go-yaml/ast"
	"github.com/goccy/go-yaml/parser"
	"github.com/rwx-research/mint-cli/internal/errors"
)

type YamlDoc struct {
	astFile *ast.File
}

func ParseYamlDoc(contents string) (*YamlDoc, error) {
	astFile, err := parser.ParseBytes([]byte(contents), parser.ParseComments)
	if err != nil {
		return nil, err
	}

	return &YamlDoc{astFile: astFile}, nil
}

func (doc *YamlDoc) Bytes() []byte {
	return []byte(doc.String())
}

func (doc *YamlDoc) String() string {
	return doc.astFile.String()
}

func (doc *YamlDoc) HasBase() bool {
	return doc.hasPath("$.base")
}

func (doc *YamlDoc) HasTasks() bool {
	return doc.hasPath("$.tasks")
}

func (doc *YamlDoc) ReadStringAtPath(yamlPath string) (string, error) {
	node, err := doc.getNodeAtPath(yamlPath)
	if err != nil {
		return "", err
	}

	return node.String(), nil
}

func (doc *YamlDoc) TryReadStringAtPath(yamlPath string) string {
	str, err := doc.ReadStringAtPath(yamlPath)
	if err != nil {
		return ""
	}
	return str
}

func (doc *YamlDoc) InsertOrUpdateBase(spec BaseLayerSpec) error {
	base := map[string]any{
		"os": spec.Os,
	}

	// Prevent unnecessary quoting of float-like tags, eg. 1.2
	if strings.Count(spec.Tag, ".") == 1 {
		parsedTag, err := strconv.ParseFloat(spec.Tag, 64)
		if err != nil {
			return err
		}
		base["tag"] = parsedTag
	} else {
		base["tag"] = spec.Tag
	}

	if spec.Arch != "" && spec.Arch != "x86_64" {
		base["arch"] = spec.Arch
	}

	if !doc.HasBase() {
		return doc.InsertBefore("$.tasks", map[string]any{
			"base": base,
		})
	} else {
		return doc.MergeAtPath("$.base", base)
	}
}

func (doc *YamlDoc) InsertBefore(beforeYamlPath string, value any) error {
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

	updatedDoc, err := ParseYamlDoc(string(result))
	if err != nil {
		return err
	}

	*doc = *updatedDoc

	return nil
}

func (doc *YamlDoc) MergeAtPath(yamlPath string, value any) error {
	p, err := yaml.PathString(yamlPath)
	if err != nil {
		panic(err)
	}

	node, err := yaml.NewEncoder(nil).EncodeToNode(value)
	if err != nil {
		return err
	}

	return p.MergeFromNode(doc.astFile, node)
}

func (doc *YamlDoc) ReplaceAtPath(yamlPath string, replacement any) error {
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

	return p.ReplaceWithNode(doc.astFile, node)
}

func (doc *YamlDoc) SetAtPath(yamlPath string, value any) error {
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

	return path.MergeFromNode(doc.astFile, node)
}

func (doc *YamlDoc) getNodeAtPath(yamlPath string) (ast.Node, error) {
	p, err := yaml.PathString(yamlPath)
	if err != nil {
		panic(err)
	}

	return p.FilterFile(doc.astFile)
}

func (doc *YamlDoc) hasPath(yamlPath string) bool {
	_, err := doc.getNodeAtPath(yamlPath)
	if err != nil {
		return false
	}

	return true
}
