package api

type MintDirectoryEntry struct {
	OriginalPath string `json:"-"`
	Path         string `json:"path"`
	Type         string `json:"type"`
	Permissions  uint32 `json:"permissions"`
	FileContents string `json:"file_contents"`
}

func (e MintDirectoryEntry) IsDir() bool {
	return e.Type == "dir"
}

func (e MintDirectoryEntry) IsFile() bool {
	return e.Type == "file"
}
