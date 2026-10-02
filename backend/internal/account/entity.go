package account

// Account 账户持久化模型；密码和令牌不会序列化到接口响应。
type Account struct {
	ID           uint   `gorm:"primaryKey" json:"id"`                          // 账户唯一 ID
	Username     string `gorm:"unique" json:"username"`                        // 唯一用户名，用于登录、展示以及评论中的 @ 提及
	Password     string `json:"-"`                                             // 注册时接收明文，入库前由 service 转为 bcrypt 哈希
	Token        string `json:"-"`                                             // 当前保存的访问令牌，用于校验登录会话
	RefreshToken string `json:"-"`                                             // 当前保存的刷新令牌，用于换取新的访问令牌
	AvatarURL    string `gorm:"type:varchar(512)" json:"avatar_url,omitempty"` // 头像图片的访问地址
	Bio          string `gorm:"type:varchar(255)" json:"bio,omitempty"`        // 个人简介
}

// CreateAccountRequest 注册请求。
type CreateAccountRequest struct {
	Username string `json:"username"` // 待注册的唯一用户名
	Password string `json:"password"` // 明文密码；service 哈希后保存，不直接入库
}

// RenameRequest 修改当前登录用户的用户名；用户 ID 从认证上下文获取。
type RenameRequest struct {
	NewUsername string `json:"new_username"` // 新的用户名；成功后需使用返回的新访问令牌
}

// FindByIDRequest 按账户 ID 查询用户。
type FindByIDRequest struct {
	ID uint `json:"id"` // 要查询的账户 ID，不是视频 ID
}

// FindByIDResponse 用户公开资料，也用于个人主页的 account 字段。
type FindByIDResponse struct {
	ID        uint   `json:"id"`                   // 账户唯一 ID
	Username  string `json:"username"`             // 当前用户名
	AvatarURL string `json:"avatar_url,omitempty"` // 头像访问地址；为空时可省略
	Bio       string `json:"bio,omitempty"`        // 个人简介；为空时可省略
}

// FindByUsernameRequest 按用户名查询用户。
type FindByUsernameRequest struct {
	Username string `json:"username"` // 要精确匹配的用户名
}

// FindByUsernameResponse 按用户名查询的公开信息结构；当前 handler 实际直接返回 Account。
type FindByUsernameResponse struct {
	ID       uint   `json:"id"`       // 匹配账户的 ID
	Username string `json:"username"` // 匹配账户的用户名
}

// ChangePasswordRequest 修改密码请求；修改成功后清除该账户的登录令牌。
type ChangePasswordRequest struct {
	Username    string `json:"username"`     // 需要修改密码的账户用户名
	OldPassword string `json:"old_password"` // 旧的明文密码，用于验证操作权限
	NewPassword string `json:"new_password"` // 新的明文密码，哈希后替换旧密码
}

// LoginRequest 登录请求。
type LoginRequest struct {
	Username string `json:"username"` // 登录账户的用户名
	Password string `json:"password"` // 明文密码，用于与数据库中的 bcrypt 哈希校验
}

// LoginResponse 登录或刷新访问令牌的响应。
type LoginResponse struct {
	Token        string `json:"token"`         // 访问令牌；调用受保护接口时放入 Authorization: Bearer 请求头
	RefreshToken string `json:"refresh_token"` // 登录时返回的刷新令牌；当前刷新接口不轮换它，响应中此字段为空
	AccountID    uint   `json:"account_id"`    // 本次登录或刷新对应的账户 ID
	Username     string `json:"username"`      // 该账户的当前用户名
}

// UpdateProfileRequest 更新当前登录用户的资料；空字符串表示不更新对应字段。
type UpdateProfileRequest struct {
	AvatarURL string `json:"avatar_url"` // 新的头像访问地址；非空时去除两端空白后更新
	Bio       string `json:"bio"`        // 新的个人简介；非空时去除两端空白后更新
}

// RefreshRequest 使用已保存的刷新令牌换取新访问令牌。
type RefreshRequest struct {
	RefreshToken string `json:"refresh_token"` // 登录时取得的刷新令牌，必须与账户当前保存的值一致
}

// GetProfileRequest 查询指定用户的个人主页。
type GetProfileRequest struct {
	AccountID uint `json:"account_id"` // 个人主页所属账户 ID，不一定是当前登录用户
}

// GetProfileResponse 个人主页的公开资料和汇总统计。
type GetProfileResponse struct {
	Account       FindByIDResponse `json:"account"`        // 账户公开资料
	VideoCount    int64            `json:"video_count"`    // 该账户发布的视频数量
	TotalLikes    int64            `json:"total_likes"`    // 该账户所有视频的累计获赞数，不是该用户点过的赞数
	FollowerCount int64            `json:"follower_count"` // 关注该账户的人数，即粉丝数
	VloggerCount  int64            `json:"vlogger_count"`  // 该账户关注的人数；Vlogger 在此表示被关注用户
}
