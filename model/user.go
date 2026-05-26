package model

type UserRole string

const (
	UserRoleGuest UserRole = "guest"
	UserRoleUser  UserRole = "user"
	UserRoleVIP   UserRole = "vip"
	UserRoleAdmin UserRole = "admin"
)

// User 系统用户。
type User struct {
	ID           string   `json:"id" gorm:"primaryKey"`
	Username     string   `json:"username" gorm:"uniqueIndex"`
	Password     string   `json:"password,omitempty"`
	Role         UserRole `json:"role"`
	Credits      int      `json:"credits"`
	ChannelName  string   `json:"channelName"`
	InviteCode   string   `json:"inviteCode" gorm:"index"`
	InviteUsedAt string   `json:"inviteUsedAt"`
	InviterID    string   `json:"inviterId"`
	CreatedAt    string   `json:"createdAt"`
	UpdatedAt    string   `json:"updatedAt"`
}

// UserList 用户分页结果。
type UserList struct {
	Items []User `json:"items"`
	Total int    `json:"total"`
}

// AuthUser 用户公开信息。
type AuthUser struct {
	ID        string   `json:"id"`
	Username  string   `json:"username"`
	Role      UserRole `json:"role"`
	Credits   int      `json:"credits"`
	CreatedAt string   `json:"createdAt"`
	UpdatedAt string   `json:"updatedAt"`
}

// AuthSession 登录会话信息。
type AuthSession struct {
	Token string   `json:"token"`
	User  AuthUser `json:"user"`
}

func PublicUser(user User) AuthUser {
	return AuthUser{
		ID:        user.ID,
		Username:  user.Username,
		Role:      user.Role,
		Credits:   user.Credits,
		CreatedAt: user.CreatedAt,
		UpdatedAt: user.UpdatedAt,
	}
}

type CreditLogType string

const (
	CreditLogTypeAdminAdjust CreditLogType = "admin_adjust"
	CreditLogTypeAIConsume   CreditLogType = "ai_consume"
	CreditLogTypeAIRefund    CreditLogType = "ai_refund"
)

// CreditLog 算力点变更日志。
type CreditLog struct {
	ID        string        `json:"id" gorm:"primaryKey"`
	UserID    string        `json:"userId" gorm:"index"`
	Type      CreditLogType `json:"type"`
	Amount    int           `json:"amount"`
	Balance   int           `json:"balance"`
	RelatedID string        `json:"relatedId"`
	Remark    string        `json:"remark"`
	Extra     string        `json:"extra" gorm:"type:text"`
	CreatedAt string        `json:"createdAt"`
}

// CreditLogList 算力点日志分页结果。
type CreditLogList struct {
	Items []CreditLog `json:"items"`
	Total int         `json:"total"`
}
