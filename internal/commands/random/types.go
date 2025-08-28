package random

type Tag struct {
	ID        int    `xml:"id,attr"`
	Name      string `xml:"name,attr"`
	Count     int    `xml:"count,attr"`
	Type      int    `xml:"type,attr"`
	Ambiguous bool   `xml:"ambiguous,attr"`
}

type Tags struct {
	Tags []Tag `xml:"tag"`
}

type Post struct {
	PreviewURL   string `json:"preview_url"`
	SampleURL    string `json:"sample_url"`
	FileURL      string `json:"file_url"`
	Directory    int    `json:"directory"`
	Hash         string `json:"hash"`
	Width        int    `json:"width"`
	Height       int    `json:"height"`
	ID           int64  `json:"id"`
	Image        string `json:"image"`
	Change       int64  `json:"change"`
	Owner        string `json:"owner"`
	ParentID     int64  `json:"parent_id"`
	Rating       string `json:"rating"`
	Sample       bool   `json:"sample"`
	SampleHeight int    `json:"sample_height"`
	SampleWidth  int    `json:"sample_width"`
	Score        int    `json:"score"`
	Tags         string `json:"tags"`
	Source       string `json:"source"`
	Status       string `json:"status"`
	HasNotes     bool   `json:"has_notes"`
	CommentCount int    `json:"comment_count"`
}
