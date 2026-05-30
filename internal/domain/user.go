package domain

// User 工作流系统交互所使用的通用解耦用户领域模型
type User struct {
	Id           int64  `json:"id"`
	DepartmentId int64  `json:"department_id"`
	Username     string `json:"username"`
	Email        string `json:"email"`
	DisplayName  string `json:"display_name"`
	Phone        string `json:"phone"`
	LarkUserId   string `json:"lark_user_id"`
	WechatUserId string `json:"wechat_user_id"`
}
