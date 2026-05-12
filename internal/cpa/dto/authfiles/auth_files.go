package authfiles

import "time"

// AuthFilesResponse 是 CPA /management/auth-files 响应 DTO。
type AuthFilesResponse struct {
	Files []AuthFile `json:"files"`
}

// AuthFileStatusResponse 是 PATCH /management/auth-files/status 响应 DTO。
type AuthFileStatusResponse struct {
	Status   string `json:"status"`
	Disabled bool   `json:"disabled"`
}

// AuthFilesDeleteResponse 是 DELETE /management/auth-files 响应 DTO。
type AuthFilesDeleteResponse struct {
	Status  string                  `json:"status"`
	Deleted int                     `json:"deleted"`
	Files   []string                `json:"files"`
	Failed  []AuthFileDeleteFailure `json:"failed"`
}

type AuthFileDeleteFailure struct {
	Name  string `json:"name"`
	Error string `json:"error"`
}

// AuthFile 是 CPA /management/auth-files 中单个 auth file 的原始响应 DTO。
type AuthFile struct {
	ID             string           `json:"id"`
	AuthIndex      string           `json:"auth_index"`
	Name           string           `json:"name"`
	Email          string           `json:"email"`
	Type           string           `json:"type"`
	Provider       string           `json:"provider"`
	Label          string           `json:"label"`
	Status         string           `json:"status"`
	StatusMessage  string           `json:"status_message"`
	Source         string           `json:"source"`
	Disabled       bool             `json:"disabled"`
	Unavailable    bool             `json:"unavailable"`
	RuntimeOnly    bool             `json:"runtime_only"`
	Success        int64            `json:"success"`
	Failed         int64            `json:"failed"`
	UpdatedAt      *time.Time       `json:"updated_at,omitempty"`
	ModTime        *time.Time       `json:"modtime,omitempty"`
	Account        string           `json:"account,omitempty"`
	Metadata       map[string]any   `json:"metadata,omitempty"`
	Attributes     map[string]any   `json:"attributes,omitempty"`
	ProjectID      string           `json:"project_id,omitempty"`
	ProjectIDCamel string           `json:"projectId,omitempty"`
	IDToken        *AuthFileIDToken `json:"id_token"`
}

// AuthFileIDToken 是 Codex auth file 的 id_token 订阅元数据 DTO。
type AuthFileIDToken struct {
	AccountID        *string    `json:"chatgpt_account_id,omitempty"`
	AccountIDCamel   *string    `json:"chatgptAccountId,omitempty"`
	ActiveStart      *time.Time `json:"chatgpt_subscription_active_start,omitempty"`
	ActiveStartCamel *time.Time `json:"chatgptSubscriptionActiveStart,omitempty"`
	ActiveUntil      *time.Time `json:"chatgpt_subscription_active_until,omitempty"`
	ActiveUntilCamel *time.Time `json:"chatgptSubscriptionActiveUntil,omitempty"`
	PlanType         *string    `json:"plan_type,omitempty"`
	PlanTypeCamel    *string    `json:"planType,omitempty"`
}
