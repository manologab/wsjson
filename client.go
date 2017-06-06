package wsjson

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const (
	// Time allowed to write a message to the peer.
	writeWait = 10 * time.Second

	// Time allowed to read the next pong message from the peer.
	pongWait = 60 * time.Second

	// Send pings to peer with this period. Must be less than pongWait.
	pingPeriod = (pongWait * 9) / 10

	// Maximum message size allowed from peer.
	maxMessageSize = 4096
)

type WsJsonClient struct {
	manager        *serviceManager
	conn           *websocket.Conn
	output         chan interface{}
	resultsMutex   sync.RWMutex
	pendingResults map[int]chan<- json.RawMessage
	idSeq          <-chan int
}

func newWsJsonClient(conn *websocket.Conn, services []interface{}) (*WsJsonClient, error) {
	if len(services) == 0 {
		return nil, errors.New("At least one service is required")
	}

	// Request Id sequence
	idSeq := make(chan int)
	go func() {
		for i := 1; ; {
			idSeq <- i
		}
	}()

	client := &WsJsonClient{
		manager:        newServiceManager(),
		conn:           conn,
		output:         make(chan interface{}, 10),
		pendingResults: make(map[int]chan<- json.RawMessage),
		idSeq:          idSeq,
	}

	for _, serv := range services {
		err := client.manager.addService(serv)
		if err != nil {
			return nil, err
		}
	}

	return client, nil
}

func (wsjc *WsJsonClient) readLoop() {
	defer func() {
		//c.hub.unregister <- c
		wsjc.conn.Close()
	}()
	wsjc.conn.SetReadLimit(maxMessageSize)
	wsjc.conn.SetReadDeadline(time.Now().Add(pongWait))
	wsjc.conn.SetPongHandler(func(string) error {
		wsjc.conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	for {
		_, message, err := wsjc.conn.NextReader()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway) {
				log.Printf("error: %v", err)
			}
			break
		}

		go wsjc.processMessage(message)
	}
}

func (wsjc *WsJsonClient) processMessage(message io.Reader) {
	response := wsjc.handleMessage(message)
	if response != nil {
		wsjc.output <- response
	}
}

// Handles a request received from the client
// returns a Response if the request is a method call
func (wsjc *WsJsonClient) handleMessage(reader io.Reader) *Response {
	var request Request
	err := json.NewDecoder(reader).Decode(&request)
	if err != nil {
		return NewErrorResponse(NewError(ErrorParse, "Parse Error"))
	}

	if request.Version != JSONRPCVersion {
		return request.makeError(ErrorInvalidRequest, "Invalid JSONRPC Version")
	}

	if request.Result != nil && request.Params != nil {
		return request.makeError(ErrorInvalidRequest, "Message can't have both 'params' and 'result' present")
	}

	/*
		if request.Result == nil && request.Params == nil {
			return request.makeError(ErrorInvalidRequest, "No 'params' or 'result' is present in message")
		}
	*/

	if request.Result != nil {
		return wsjc.handleResult(request)
	} else {
		return wsjc.handleRequest(request)
	}

}

func (wsjc *WsJsonClient) handleRequest(request Request) *Response {
	result, err := wsjc.manager.callMethod(request.Method, request.Params)
	if err != nil {
		if jsonError, ok := err.(*Error); ok {
			response := NewErrorResponse(jsonError)
			response.Id = request.Id
			return response
		} else {
			return request.makeError(ErrorInternalError, err.Error())
		}
	}

	if result != nil {
		return &Response{
			Version: JSONRPCVersion,
			Result:  result,
			Id:      request.Id,
		}
	} else {
		return nil
	}
}

func (wsjc *WsJsonClient) handleResult(request Request) *Response {
	idRaw := request.Id
	if idRaw == nil {
		log.Printf("Result with null id received, %s", &request)
		return nil
	}

	idFloat, ok := idRaw.(float64)
	if !ok {
		log.Printf("Result id must be an integer: %v", idRaw)
		return nil
	}

	id := int(idFloat)

	ch := wsjc.removePendingResult(id)
	if ch == nil {
		log.Printf("No previous request found for result.id:%d, request: '%s'", id, &request)
	} else {
		ch <- request.Result
		close(ch)
	}

	return nil
}

func (wsjc *WsJsonClient) addPendingResult(id int, ch chan<- json.RawMessage) {
	wsjc.resultsMutex.Lock()
	defer wsjc.resultsMutex.Unlock()
	wsjc.pendingResults[id] = ch
}

func (wsjc *WsJsonClient) getPendingResult(id int) chan<- json.RawMessage {
	wsjc.resultsMutex.RLock()
	defer wsjc.resultsMutex.RUnlock()
	return wsjc.pendingResults[id]
}

func (wsjc *WsJsonClient) removePendingResult(id int) chan<- json.RawMessage {
	wsjc.resultsMutex.Lock()
	defer wsjc.resultsMutex.Unlock()
	ch := wsjc.pendingResults[id]
	if ch != nil {
		delete(wsjc.pendingResults, id)
	}
	return ch
}

// CallMethod sends a JSON-RPC request to the peer.
// Returns a channel where the result of the call will be sent when it arrives.
// An error is returned if there is a problem marshalling the param to JSON
func (wsjc *WsJsonClient) CallMethod(name string, params interface{}) (<-chan json.RawMessage, error) {
	return wsjc.SendMessage(name, params, true)
}

func (wsjc *WsJsonClient) SendEvent(name string, params interface{}) error {
	_, err := wsjc.SendMessage(name, params, false)
	return err
}

// Sends a JSON RPC message to the peer
func (wsjc *WsJsonClient) SendMessage(name string, params interface{}, isMethod bool) (ch <-chan json.RawMessage, err error) {
	rawParams, err := json.Marshal(params)
	if err != nil {
		return
	}

	request := &Request{
		Version: JSONRPCVersion,
		Method:  name,
		Params:  rawParams,
	}

	if isMethod {
		id := <-wsjc.idSeq
		request.Id = id
		chr := make(chan json.RawMessage)
		wsjc.addPendingResult(id, chr)
		ch = chr
	}

	wsjc.output <- request
	return
}

func (wsjc *WsJsonClient) serve() {
	go wsjc.readLoop()

}
