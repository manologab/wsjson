package wsjson

import (
	"encoding/json"
	"errors"
	"io"
	"log"
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
	manager *serviceManager
	conn    *websocket.Conn
	output  chan interface{}
}

func newWsJsonClient(conn *websocket.Conn, services []interface{}) (*WsJsonClient, error) {
	if len(services) == 0 {
		return nil, errors.New("At least one service is required")
	}

	client := &WsJsonClient{
		manager: newServiceManager(),
		conn:    conn,
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

	// params, err := request.paramsAsArray()
	// if err != nil {
	// 	return request.makeError(ErrorInvalidParams, err.Error())
	// }

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

func (wsjc *WsJsonClient) serve() {
	go wsjc.readLoop()

}
