package gerrit

import (
	"strings"
	"time"
)

const timestampLayout = "2006-01-02 15:04:05.000000000"

type Timestamp struct{ time.Time }

func (t *Timestamp) UnmarshalJSON(b []byte) error {
	s := strings.Trim(string(b), `"`)
	if s == "" || s == "null" {
		return nil
	}
	parsed, err := time.ParseInLocation(timestampLayout, s, time.UTC)
	if err != nil {
		return err
	}
	t.Time = parsed
	return nil
}

type AccountInfo struct {
	AccountID   int    `json:"_account_id"`
	Name        string `json:"name"`
	DisplayName string `json:"display_name"`
	Email       string `json:"email"`
	Username    string `json:"username"`
}

func (a AccountInfo) Label() string {
	switch {
	case a.DisplayName != "":
		return a.DisplayName
	case a.Name != "":
		return a.Name
	case a.Username != "":
		return a.Username
	case a.Email != "":
		return a.Email
	}
	return "unknown"
}

type ApprovalInfo struct {
	AccountInfo
	Value                int        `json:"value"`
	PermittedVotingRange *VoteRange `json:"permitted_voting_range,omitempty"`
	Date                 Timestamp  `json:"date"`
	Tag                  string     `json:"tag"`
}

type VoteRange struct {
	Min int `json:"min"`
	Max int `json:"max"`
}

type LabelInfo struct {
	Optional     bool              `json:"optional"`
	Description  string            `json:"description"`
	Approved     *AccountInfo      `json:"approved"`
	Rejected     *AccountInfo      `json:"rejected"`
	Recommended  *AccountInfo      `json:"recommended"`
	Disliked     *AccountInfo      `json:"disliked"`
	Blocking     bool              `json:"blocking"`
	Value        int               `json:"value"`
	DefaultValue int               `json:"default_value"`
	Votes        []int             `json:"votes"`
	All          []ApprovalInfo    `json:"all"`
	Values       map[string]string `json:"values"`
}

type AttentionSetInfo struct {
	Account    AccountInfo `json:"account"`
	LastUpdate Timestamp   `json:"last_update"`
	Reason     string      `json:"reason"`
}

type GitPersonInfo struct {
	Name  string    `json:"name"`
	Email string    `json:"email"`
	Date  Timestamp `json:"date"`
	TZ    int       `json:"tz"`
}

type CommitInfo struct {
	Commit    string        `json:"commit"`
	Parents   []CommitInfo  `json:"parents"`
	Author    GitPersonInfo `json:"author"`
	Committer GitPersonInfo `json:"committer"`
	Subject   string        `json:"subject"`
	Message   string        `json:"message"`
}

type FileInfo struct {
	Status        string `json:"status"`
	Binary        bool   `json:"binary"`
	OldPath       string `json:"old_path"`
	LinesInserted int    `json:"lines_inserted"`
	LinesDeleted  int    `json:"lines_deleted"`
	SizeDelta     int64  `json:"size_delta"`
	Size          int64  `json:"size"`
}

type DiffContent struct {
	A    []string `json:"a"`
	B    []string `json:"b"`
	AB   []string `json:"ab"`
	Skip int      `json:"skip"`
}

type DiffInfo struct {
	ChangeType string        `json:"change_type"`
	Binary     bool          `json:"binary"`
	Content    []DiffContent `json:"content"`
}

type FetchInfo struct {
	URL      string            `json:"url"`
	Ref      string            `json:"ref"`
	Commands map[string]string `json:"commands"`
}

type RevisionInfo struct {
	Kind        string               `json:"kind"`
	Number      int                  `json:"_number"`
	Created     Timestamp            `json:"created"`
	Uploader    AccountInfo          `json:"uploader"`
	Ref         string               `json:"ref"`
	Fetch       map[string]FetchInfo `json:"fetch"`
	Commit      *CommitInfo          `json:"commit"`
	Files       map[string]FileInfo  `json:"files"`
	Description string               `json:"description"`
}

type ChangeMessageInfo struct {
	ID             string       `json:"id"`
	Author         *AccountInfo `json:"author"`
	RealAuthor     *AccountInfo `json:"real_author"`
	Date           Timestamp    `json:"date"`
	Message        string       `json:"message"`
	Tag            string       `json:"tag"`
	RevisionNumber int          `json:"_revision_number"`
}

type SubmitRequirementResultInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Status      string `json:"status"`
	IsLegacy    bool   `json:"is_legacy"`
}

type ChangeInfo struct {
	ID                     string                        `json:"id"`
	Project                string                        `json:"project"`
	Branch                 string                        `json:"branch"`
	Topic                  string                        `json:"topic"`
	AttentionSet           map[string]AttentionSetInfo   `json:"attention_set"`
	Hashtags               []string                      `json:"hashtags"`
	ChangeID               string                        `json:"change_id"`
	Subject                string                        `json:"subject"`
	Status                 string                        `json:"status"`
	Created                Timestamp                     `json:"created"`
	Updated                Timestamp                     `json:"updated"`
	Submitted              Timestamp                     `json:"submitted"`
	Submitter              *AccountInfo                  `json:"submitter"`
	Starred                bool                          `json:"starred"`
	Reviewed               bool                          `json:"reviewed"`
	Mergeable              *bool                         `json:"mergeable"`
	Submittable            bool                          `json:"submittable"`
	Insertions             int                           `json:"insertions"`
	Deletions              int                           `json:"deletions"`
	TotalCommentCount      int                           `json:"total_comment_count"`
	UnresolvedCommentCount int                           `json:"unresolved_comment_count"`
	Number                 int                           `json:"_number"`
	Owner                  AccountInfo                   `json:"owner"`
	Labels                 map[string]LabelInfo          `json:"labels"`
	PermittedLabels        map[string][]string           `json:"permitted_labels"`
	Reviewers              map[string][]AccountInfo      `json:"reviewers"`
	Messages               []ChangeMessageInfo           `json:"messages"`
	CurrentRevisionNumber  int                           `json:"current_revision_number"`
	CurrentRevision        string                        `json:"current_revision"`
	Revisions              map[string]RevisionInfo       `json:"revisions"`
	SubmitRequirements     []SubmitRequirementResultInfo `json:"submit_requirements"`
	MoreChanges            bool                          `json:"_more_changes"`
	IsPrivate              bool                          `json:"is_private"`
	WorkInProgress         bool                          `json:"work_in_progress"`
	HasReviewStarted       bool                          `json:"has_review_started"`
	RevertOf               int                           `json:"revert_of"`
}

func (c *ChangeInfo) CurrentRevisionInfo() *RevisionInfo {
	if c.Revisions == nil {
		return nil
	}
	if rev, ok := c.Revisions[c.CurrentRevision]; ok {
		return &rev
	}
	return nil
}
