package schema

type CreateConversationRequest struct {
	Title string `json:"title" binding:"required"`
}

type CreateConversationResponse struct {
	ID    uint   `json:"id"`
	Title string `json:"title"`
}

type ConversationInfo struct {
	ID        uint   `json:"id"`
	Title     string `json:"title"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

type GetConversationsResponse struct {
	Conversations []ConversationInfo `json:"conversations"`
	Total         int64              `json:"total"`
	Page          int                `json:"page"`
	PageSize      int                `json:"pageSize"`
}

type GetConversationResponse struct {
	ID               uint   `json:"id"`
	Title            string `json:"title"`
	MessageCount     int    `json:"messageCount"`
	FreeMessageCount int    `json:"freeMessageCount"`
	CreatedAt        string `json:"createdAt"`
	UpdatedAt        string `json:"updatedAt"`
}
