package schema

type ChatRequest struct {
	Message      string `json:"message" binding:"required"`
	SessionId    uint   `json:"sessionId" binding:"required"`
	ParentId     uint   `json:"parentId"`
	Title        string `json:"title"`
	MessageOrder uint   `json:"messageOrder"`
	ImageUrl     string `json:"imageUrl"`
}
