package api

type TaskDefinition struct {
	Path         string
	FileContents string // This type is expected by cloud
}
