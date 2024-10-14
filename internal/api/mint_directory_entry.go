package api

type MintDirectoryEntry struct {
	OriginalPath string `json:"-"`
	Path         string `json:"path"`
	Type         string `json:"type"`
	Permissions  uint32 `json:"permissions"`
	FileContents string `json:"file_contents"`
}
