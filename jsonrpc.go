package wsjson

import (
	"encoding/json"
	"fmt"
)

const (
	JSONRPCVersion      string = "2.0"
	ErrorParse          int    = -32700
	ErrorInvalidRequest int    = -32600
	ErrorMethodNotFound int    = -32601
	ErrorInvalidParams  int    = -32602
	ErrorInternalError  int    = -32603
)

type Error struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

type Request struct {
	Version string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
	Id      interface{}     `json:"id,omitempty"`
}

type Response struct {
	Version string      `json:"jsonrpc"`
	Result  interface{} `json:"result"`
	Err     *Error      `json:"error,omitempty"`
	Id      interface{} `json:"id"`
}

func NewError(code int, message string, a ...interface{}) *Error {
	return NewErrorWithData(code, fmt.Sprintf(message, a...), nil)
}

func NewErrorWithData(code int, message string, data interface{}) *Error {
	resp := Error{
		Code:    code,
		Message: message,
	}

	if data != nil {
		resp.Data = data
	}

	return &resp
}

func (je *Error) Error() string {
	return je.Message
}

func NewErrorResponse(error *Error) *Response {
	return &Response{
		Version: JSONRPCVersion,
		Err:     error,
		Id:      nil,
	}
}

// Creates an error tu return for a given request
func (req *Request) makeError(code int, message string, a ...interface{}) *Response {
	err := NewError(code, message, a...)

	response := &Response{
		Version: JSONRPCVersion,
		Err:     err,
		Id:      req.Id,
	}
	return response
}
