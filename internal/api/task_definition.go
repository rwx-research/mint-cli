package api

type TaskDefinition struct {
	Path         string `json:"path"`
	FileContents string `json:"file_contents"`
}
