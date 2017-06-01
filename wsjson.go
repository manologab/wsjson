package wsjson

import (
	"log"
	"net/http"

	"github.com/gorilla/websocket"
)

const (
	defReadBufferSize  = 1024
	defWriteBufferSize = 1024
)

// Provide API's for each WS connection
type ApiFactory func(http.ResponseWriter, *http.Request) []interface{}

// Main Websocket handler
type WsJson struct {
	apiFactory ApiFactory

	// websocket upgrader, can be overwriten by the user
	wsUpgrader *websocket.Upgrader
}

// Get the websocket upgrader
func (wsj *WsJson) upgrader() *websocket.Upgrader {
	if wsj.wsUpgrader == nil {
		wsj.wsUpgrader = &websocket.Upgrader{
			ReadBufferSize:  defReadBufferSize,
			WriteBufferSize: defWriteBufferSize,
		}
	}
	return wsj.wsUpgrader
}

// Set a custom websocket upgrader
func (wsj *WsJson) SetUpgrader(upgrader *websocket.Upgrader) {
	wsj.wsUpgrader = upgrader
}

func (wsj *WsJson) SetApiFactory(factory ApiFactory) {
	wsj.apiFactory = factory
}

// Implementation of net.http.Handler to manage websocket endpoints
// this method must be registered with http.Handle
func (wsj *WsJson) Handle(w http.ResponseWriter, r *http.Request) {
	// Api factory is required
	if wsj.apiFactory == nil {
		log.Fatalf("No API factory defined")
		return // TODO: return error
	}

	apiObjects := wsj.apiFactory(w, r)
	if apiObjects == nil {
		// apiFactory should have handled the response
		return
	}

	// Upgrade connection to websocket
	conn, err := wsj.upgrader().Upgrade(w, r, nil)
	if err != nil {
		log.Println(err)
		return
	}

	client, err := newWsJsonClient(conn, apiObjects)
	if err != nil {
		log.Printf("Error creating client: %v\n", err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	client.serve()

}
