package models

import (
	"time"
)

// RequestType are network log types
type RequestType int

const (
	HTTP RequestType = 0
	WS
)

// Result is a Gowitness result
type Result struct {
	ID uint `json:"id" gorm:"primarykey"`

	URL                   string    `json:"url"`
	ProbedAt              time.Time `json:"probed_at"`
	FinalURL              string    `json:"final_url"`
	ResponseCode          int       `json:"response_code"`
	ResponseReason        string    `json:"response_reason"`
	Protocol              string    `json:"protocol"`
	ContentLength         int64     `json:"content_length"`
	HTML                  string    `json:"html" gorm:"type:longtext;index:,length:191"`
	Title                 string    `json:"title" gorm:"index"`
	PerceptionHash        string    `json:"perception_hash" gorm:"index"`
	PerceptionHashGroupId uint      `json:"perception_hash_group_id" gorm:"index"`
	Screenshot            string    `json:"screenshot"`

	// Name of the screenshot file
	Filename string `json:"file_name"`
	IsPDF    bool   `json:"is_pdf"`

	// Failed flag set if the result should be considered failed
	Failed       bool   `json:"failed"`
	FailedReason string `json:"failed_reason"`

	TLS          TLS          `json:"tls" gorm:"constraint:OnDelete:CASCADE"`
	Technologies []Technology `json:"technologies" gorm:"constraint:OnDelete:CASCADE"`

	Headers []Header     `json:"headers" gorm:"constraint:OnDelete:CASCADE"`
	Network []NetworkLog `json:"network" gorm:"constraint:OnDelete:CASCADE"`
	Console []ConsoleLog `json:"console" gorm:"constraint:OnDelete:CASCADE"`
	Cookies []Cookie     `json:"cookies" gorm:"constraint:OnDelete:CASCADE"`

	// Additive clustering and deduplication metadata. These fields are
	// intentionally optional so older JSON Lines, CSV and database reports
	// remain readable.
	Favicon             string   `json:"favicon,omitempty"`
	FaviconURL          string   `json:"favicon_url,omitempty"`
	FaviconHash         string   `json:"favicon_hash,omitempty" gorm:"index"`
	ScreenshotHash      string   `json:"screenshot_hash,omitempty" gorm:"index"`
	TitleNormalized     string   `json:"title_normalized,omitempty" gorm:"index:,length:191"`
	ScreenshotClusterID uint     `json:"screenshot_cluster_id,omitempty" gorm:"index"`
	ClusterID           uint     `json:"cluster_id,omitempty" gorm:"index"`
	FaviconClusterID    uint     `json:"favicon_cluster_id,omitempty" gorm:"index"`
	TitleClusterID      uint     `json:"title_cluster_id,omitempty" gorm:"index"`
	DuplicateOfID       uint     `json:"duplicate_of_id,omitempty" gorm:"index"`
	CanonicalURL        string   `json:"canonical_url,omitempty"`
	IsDuplicate         bool     `json:"is_duplicate,omitempty" gorm:"index"`
	DuplicateCount      int      `json:"duplicate_count,omitempty"`
	DuplicateURLs       []string `json:"duplicate_urls,omitempty" gorm:"serializer:json"`

	// Compatibility aliases for codebases/tests using the repository's
	// older "Id" capitalisation. They are not serialised or migrated.
	ScreenshotClusterId uint `json:"-" gorm:"-"`
	ClusterId           uint `json:"-" gorm:"-"`
	FaviconClusterId    uint `json:"-" gorm:"-"`
	TitleClusterId      uint `json:"-" gorm:"-"`
	DuplicateOfId       uint `json:"-" gorm:"-"`
}

// SyncClusterAliases keeps the exported ID/Id spelling variants aligned.
func (r *Result) SyncClusterAliases() {
	if r == nil {
		return
	}

	if r.ScreenshotClusterID == 0 && r.ScreenshotClusterId != 0 {
		r.ScreenshotClusterID = r.ScreenshotClusterId
	}
	if r.ScreenshotClusterId == 0 && r.ScreenshotClusterID != 0 {
		r.ScreenshotClusterId = r.ScreenshotClusterID
	}

	if r.ClusterID == 0 && r.ClusterId != 0 {
		r.ClusterID = r.ClusterId
	}
	if r.ClusterId == 0 && r.ClusterID != 0 {
		r.ClusterId = r.ClusterID
	}

	if r.FaviconClusterID == 0 && r.FaviconClusterId != 0 {
		r.FaviconClusterID = r.FaviconClusterId
	}
	if r.FaviconClusterId == 0 && r.FaviconClusterID != 0 {
		r.FaviconClusterId = r.FaviconClusterID
	}

	if r.TitleClusterID == 0 && r.TitleClusterId != 0 {
		r.TitleClusterID = r.TitleClusterId
	}
	if r.TitleClusterId == 0 && r.TitleClusterID != 0 {
		r.TitleClusterId = r.TitleClusterID
	}

	if r.DuplicateOfID == 0 && r.DuplicateOfId != 0 {
		r.DuplicateOfID = r.DuplicateOfId
	}
	if r.DuplicateOfId == 0 && r.DuplicateOfID != 0 {
		r.DuplicateOfId = r.DuplicateOfID
	}
}

func (r *Result) HeaderMap() map[string][]string {
	headersMap := make(map[string][]string)

	for _, header := range r.Headers {
		headersMap[header.Key] = []string{header.Value}
	}

	return headersMap
}

type TLS struct {
	ID       uint `json:"id" gorm:"primarykey"`
	ResultID uint `json:"resultid"`

	Protocol                 string       `json:"protocol"`
	KeyExchange              string       `json:"key_exchange"`
	Cipher                   string       `json:"cipher"`
	SubjectName              string       `json:"subject_name"`
	SanList                  []TLSSanList `json:"san_list" gorm:"constraint:OnDelete:CASCADE"`
	Issuer                   string       `json:"issuer"`
	ValidFrom                time.Time    `json:"valid_from"`
	ValidTo                  time.Time    `json:"valid_to"`
	ServerSignatureAlgorithm int64        `json:"server_signature_algorithm"`
	EncryptedClientHello     bool         `json:"encrypted_client_hello"`
}

type TLSSanList struct {
	ID    uint `json:"id" gorm:"primarykey"`
	TLSID uint `json:"tls_id"`

	Value string `json:"value"`
}

type Technology struct {
	ID       uint `json:"id" gorm:"primarykey"`
	ResultID uint `json:"result_id"`

	Value string `json:"value" gorm:"index"`
}

type Header struct {
	ID       uint `json:"id" gorm:"primarykey"`
	ResultID uint `json:"result_id"`

	Key   string `json:"key"`
	Value string `json:"value" gorm:"type:longtext;index:,length:191"`
}

type NetworkLog struct {
	ID       uint `json:"id" gorm:"primarykey"`
	ResultID uint `json:"result_id"`

	RequestType RequestType `json:"request_type"`
	StatusCode  int64       `json:"status_code"`
	URL         string      `json:"url"`
	RemoteIP    string      `json:"remote_ip"`
	MIMEType    string      `json:"mime_type"`
	Time        time.Time   `json:"time"`
	Content     []byte      `json:"content"`
	Error       string      `json:"error"`
}

type ConsoleLog struct {
	ID       uint `json:"id" gorm:"primarykey"`
	ResultID uint `json:"result_id"`

	Type  string `json:"type"`
	Value string `json:"value" gorm:"type:longtext;index:,length:191"`
}

type Cookie struct {
	ID       uint `json:"id" gorm:"primarykey"`
	ResultID uint `json:"result_id"`

	Name         string    `json:"name"`
	Value        string    `json:"value"`
	Domain       string    `json:"domain"`
	Path         string    `json:"path"`
	Expires      time.Time `json:"expires"`
	Size         int64     `json:"size"`
	HTTPOnly     bool      `json:"http_only"`
	Secure       bool      `json:"secure"`
	Session      bool      `json:"session"`
	Priority     string    `json:"priority"`
	SourceScheme string    `json:"source_scheme"`
	SourcePort   int64     `json:"source_port"`
}
