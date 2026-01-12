package types

type Request struct {
	ID      string                 `json:"id"`
	Action  string                 `json:"action"`
	Account string                 `json:"account,omitempty"`
	Params  map[string]interface{} `json:"params"`
}

type Response struct {
	ID       string   `json:"id"`
	Ok       bool     `json:"ok"`
	Data     any      `json:"data,omitempty"`
	Warnings []string `json:"warnings,omitempty"`
	Error    *Error   `json:"error,omitempty"`
}

type Error struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details string `json:"details,omitempty"`
}

func NewError(code, message, details string) *Error {
	return &Error{Code: code, Message: message, Details: details}
}
