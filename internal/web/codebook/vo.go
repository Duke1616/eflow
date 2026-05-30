package codebook

type CreateReq struct {
	Name       string `json:"name"`
	Owner      string `json:"owner"`
	Code       string `json:"code"`
	Language   string `json:"language"`
	Identifier string `json:"identifier"`
}

type DetailReq struct {
	Id int64 `json:"id"`
}

type UpdateReq struct {
	Id       int64  `json:"id"`
	Name     string `json:"name"`
	Owner    string `json:"owner"`
	Code     string `json:"code"`
	Language string `json:"language"`
}

type DeleteReq struct {
	Id int64 `json:"id"`
}

type Page struct {
	Offset int64 `json:"offset,omitempty"`
	Limit  int64 `json:"limit,omitempty"`
}

type ListReq struct {
	Page
}

type CodebookVO struct {
	Id         int64  `json:"id"`
	Name       string `json:"name"`
	Owner      string `json:"owner"`
	Identifier string `json:"identifier"`
	Code       string `json:"code"`
	Language   string `json:"language"`
	Secret     string `json:"secret"`
}

type ListCodebooksResp struct {
	Total     int64        `json:"total"`
	Codebooks []CodebookVO `json:"codebooks"`
}
