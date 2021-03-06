package wsjson

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"
)

const (
	errVoldemor int = 100
	errValkyrie int = 101
)

type AllTypes struct {
	Number int     `json:"number"`
	Name   string  `json:"name"`
	Price  float64 `json:"price"`
	Flag   bool    `json:"flag"`
}

type SimpleService struct {
	calls     int
	lastEvent string
}

func (*SimpleService) ApiEcho(message string) (string, error) {
	if message == "Voldemor" {
		return "", NewError(errVoldemor, "Don't mention his name")
	}

	if message == "Valkyrie" {
		data := AllTypes{19420720, "Führer", 2.66, true}
		return "", NewErrorWithData(errValkyrie, "Secret data", data)
	}
	return message, nil
}

func (*SimpleService) ApiDouble(number int, name string, price float64, flag bool) (*AllTypes, error) {
	if number == 3141592 {
		return nil, errors.New("An artifitial error")
	}

	return &AllTypes{
		Number: number * 2,
		Name:   name + name,
		Price:  price * 2,
		Flag:   !flag,
	}, nil

}

func (ss *SimpleService) ApiEvent(event string) {
	ss.lastEvent = event

}

func (ss *SimpleService) ApiAnObject(param AllTypes) (int, error) {
	return param.Number, nil
}

func (ss *SimpleService) ApiAnObjectPtr(param *AllTypes) (string, error) {
	return fmt.Sprintf("%+v", param), nil
}

func (ss *SimpleService) ApiAllTypesPtr(number *int, name *string, price *float64, flag *bool) (AllTypes, error) {
	return AllTypes{
		Number: *number,
		Name:   *name,
		Price:  *price,
		Flag:   *flag,
	}, nil
}

func (ss *SimpleService) ApiAnArray(params []string) (int, error) {
	c := 0
	for _, s := range params {
		c += len(s)
	}
	return c, nil
}

// API that provides a custom name and method prefix
type NamedPrefixService struct {
	lastEvent *AllTypes
}

func (*NamedPrefixService) WsName() string {
	return "napre"
}

func (*NamedPrefixService) WsPrefix() string {
	return "Serv"
}

func (*NamedPrefixService) ServFields2Obj(number int, name string, price float64, flag bool) (AllTypes, error) {
	return AllTypes{
		Number: number,
		Name:   name,
		Price:  price,
		Flag:   flag,
	}, nil
}

func (*NamedPrefixService) ServObj2String(param *AllTypes) (string, error) {
	return fmt.Sprintf("%+v", param), nil
}

func (ns *NamedPrefixService) ServEvent(param *AllTypes) {
	ns.lastEvent = param
}

// API that provides the exposed methods explicitely
type MethodProviderService struct {
}

func (*MethodProviderService) WsName() string {
	return "methods"
}

func (mp *MethodProviderService) WsMethods() map[string]string {
	return map[string]string{
		"secret_of_life": "GetTheSecretOfLife",
	}
}

func (mp *MethodProviderService) GetTheSecretOfLife() (int, error) {
	return 42, nil
}

// Create a client instance with all testing service
func createClient() (
	client *WsJsonClient, simpleService *SimpleService,
	namedService *NamedPrefixService, methodService *MethodProviderService,
	err error) {
	simpleService = &SimpleService{calls: 0}
	namedService = &NamedPrefixService{}
	methodService = &MethodProviderService{}
	client, err = newWsJsonClient(nil, []interface{}{simpleService, namedService, methodService})
	if err != nil {
		return
	}

	if client.manager.numServices() != 3 {
		err = fmt.Errorf("Should had 3 services registered, found: %d", client.manager.numServices())
	}

	expedtedMethods := 11
	if client.manager.numMethods() != expedtedMethods {
		err = fmt.Errorf("Should had %d methods registered, found: %d", expedtedMethods, client.manager.numMethods())
	}

	return
}

//Test client with all kinds of API objects
func TestSimpleService(t *testing.T) {
	client, simpleService, namedService, _, err := createClient()
	if err != nil {
		t.Fatal(err)
	}

	var testCases = []struct {
		msg     string
		errCode int
		errMsg  string
		errData interface{}
		result  interface{}
	}{
		{"xxx", ErrorParse, "Parse Error", nil, nil},
		{`{"jsonrpc": "1.0", "method": "testing", "params": [], "id": %idx%}`,
			ErrorInvalidRequest, "Invalid JSONRPC Version", nil, nil},
		{`{"jsonrpc": "2.0", "method": "testing", "params": [], "result":[], "id": %idx%}`,
			ErrorInvalidRequest, "Message can't have both", nil, nil},
		{`{"jsonrpc": "2.0", "method": "testing", "params": [44], "id": %idx%}`,
			ErrorMethodNotFound, "Invalid method name", nil, nil},
		{`{"jsonrpc": "2.0", "method": "yada.yada", "params": [44], "id": %idx%}`,
			ErrorMethodNotFound, "API not found: yada", nil, nil},
		{`{"jsonrpc": "2.0", "method": "SimpleService.yada", "params": [44], "id": %idx%}`,
			ErrorMethodNotFound, "API SimpleService doesn't have the yada method", nil, nil},

		{`{"jsonrpc": "2.0", "method": "SimpleService.AnObject", "params": 44, "id": %idx%}`,
			ErrorInvalidParams, "Params must be an object", nil, nil},

		{`{"jsonrpc": "2.0", "method": "SimpleService.AnObject", "params": [44], "id": %idx%}`,
			ErrorInvalidParams, "Params must be an object", nil, nil},

		{`{"jsonrpc": "2.0", "method": "SimpleService.AnObject", "params": {"number": 1979, "name": "Jerome", "flag": true, "price": 1.99}, "id": %idx%}`,
			0, "", nil, 1979},
		{`{"jsonrpc": "2.0", "method": "SimpleService.AnObject", "params": {}, "id": %idx%}`,
			0, "", nil, 0},

		{`{"jsonrpc": "2.0", "method": "SimpleService.AnObjectPtr", "params": {"number": 1979, "name": "Jerome", "flag": true, "price": 1.99}, "id": %idx%}`,
			0, "", nil, fmt.Sprintf("%+v", &AllTypes{1979, "Jerome", 1.99, true})},

		{`{"jsonrpc": "2.0", "method": "SimpleService.Echo", "params": 444, "id": %idx%}`,
			ErrorInvalidParams, "Params must be an array", nil, nil},

		{`{"jsonrpc": "2.0", "method": "SimpleService.Echo", "params": [], "id": %idx%}`,
			ErrorInvalidParams, "Wrong number of arguments", nil, nil},
		{`{"jsonrpc": "2.0", "method": "SimpleService.Echo", "params": [555444], "id": %idx%}`,
			ErrorInvalidParams, "Unable to decode parameter 0", nil, nil},
		{`{"jsonrpc": "2.0", "method": "SimpleService.Echo", "params": ["Voldemor"], "id": %idx%}`,
			errVoldemor, "Don't mention his name", nil, nil},
		{`{"jsonrpc": "2.0", "method": "SimpleService.Echo", "params": ["Valkyrie"], "id": %idx%}`,
			errValkyrie, "Secret data", AllTypes{19420720, "Führer", 2.66, true}, nil},
		{`{"jsonrpc": "2.0", "method": "SimpleService.Echo", "params": ["Mirror"], "id": %idx%}`,
			0, "", nil, "Mirror"},

		{`{"jsonrpc": "2.0", "method": "SimpleService.Double", "params": [3141592, "Vito", 3.141592, false], "id": %idx%}`,
			ErrorInternalError, "An artifitial error", nil, nil},
		{`{"jsonrpc": "2.0", "method": "SimpleService.Double", "params": [512, "Vito", 5.12, false], "id": %idx%}`,
			0, "", nil, &AllTypes{1024, "VitoVito", 10.24, true}},

		{`{"jsonrpc": "2.0", "method": "SimpleService.AllTypesPtr", "params": [512, "Vito", 5.12, false], "id": %idx%}`,
			0, "", nil, AllTypes{512, "Vito", 5.12, false}},

		{`{"jsonrpc": "2.0", "method": "SimpleService.AnArray", "params": [111], "id": %idx%}`,
			ErrorInvalidParams, "Unable to decode parameter 0", nil, nil},

		{`{"jsonrpc": "2.0", "method": "SimpleService.AnArray", "params": [[3, 2, 1]], "id": %idx%}`,
			ErrorInvalidParams, "Unable to decode parameter 0", nil, nil},

		{`{"jsonrpc": "2.0", "method": "SimpleService.AnArray", "params": [["a", "bb", "ccc"]], "id": %idx%}`,
			0, "", nil, 6},

		// Named services
		{`{"jsonrpc": "2.0", "method": "napre.Fields2Obj", "params": [1789, "Bastille Day", 1.789, true]}`,
			0, "", nil, AllTypes{1789, "Bastille Day", 1.789, true}},
		{`{"jsonrpc": "2.0", "method": "napre.Obj2String", "params": {"number":1789, "name":"Bastille Day", "price":1.789, "flag":true}}`,
			0, "", nil, fmt.Sprintf("%+v", &AllTypes{1789, "Bastille Day", 1.789, true})},

		//API implementing MethodsProvider
		// no params method
		{`{"jsonrpc": "2.0", "method": "methods.secret_of_life", "params": [], "id":%idx%}`,
			0, "", nil, 42},
		{`{"jsonrpc": "2.0", "method": "methods.secret_of_life", "params": [111], "id":%idx%}`,
			ErrorInvalidParams, "Wrong number of arguments", nil, nil},
		{`{"jsonrpc": "2.0", "method": "methods.secret_of_life", "params": null, "id":%idx%}`,
			0, "", nil, 42},
		{`{"jsonrpc": "2.0", "method": "methods.secret_of_life", "id":%idx%}`,
			0, "", nil, 42},

		// errors in events should returm responses
		{`{"jsonrpc": "2.0", "method": "SimpleService.Event", "params": []}`,
			ErrorInvalidParams, "Wrong number of arguments", nil, nil},
	}

	for i, tc := range testCases {

		checkIdx := false
		if strings.Contains(tc.msg, "%idx%") {
			tc.msg = strings.Replace(tc.msg, "%idx%", strconv.Itoa(i), 1)
			checkIdx = true
		}

		msg := strings.NewReader(tc.msg)
		resp := client.handleMessage(msg)

		if resp.Version != JSONRPCVersion {
			t.Errorf("Invalid JSONRPC Version for '%s', got: '%s'", tc.msg, resp.Version)
		}

		if checkIdx && (resp.Id == nil || int(resp.Id.(float64)) != i) {
			t.Errorf("Invalid Id for '%s', expected:%d, got:%v", tc.msg, i, resp.Id)
		}

		if !checkIdx && resp.Id != nil {
			t.Errorf("No Id expected for '%s', got:%#v", tc.msg, resp.Id)
		}

		if resp.Result != nil && resp.Err != nil {
			t.Errorf("Response should not have result and error set, request: '%s', response: %#v", tc.msg, resp)
		}

		if !reflect.DeepEqual(resp.Result, tc.result) {
			t.Errorf("Invalid result for '%s', expected: %#v, got: %#v", tc.msg, tc.result, resp.Result)
		}

		if tc.errMsg == "" {
			if resp.Err != nil {
				t.Errorf("No error was expected, got: %#v", resp.Err)
			}

			// stop error checks
			continue
		}

		if resp.Err == nil {
			t.Errorf("An error was expected for '%s' none received, expected code:%d, expected message: '%s'", tc.msg, tc.errCode, tc.errMsg)
			continue
		}

		// Error checks

		if resp.Err.Code != tc.errCode {
			t.Errorf("Invalid error code for '%s', expected:%d, got:%d", tc.msg, tc.errCode, resp.Err.Code)
		}

		if !strings.Contains(resp.Err.Message, tc.errMsg) {
			t.Errorf("Invalid error message for '%s', expected:'%s', got:'%s'", tc.msg, tc.errMsg, resp.Err.Message)
		}

		if !reflect.DeepEqual(resp.Err.Data, tc.errData) {
			t.Errorf("Invalid Error data, expected: %#v, got: %#v", tc.errData, resp.Err.Data)
		}

	}

	// test a simple event success
	msg := `{"jsonrpc": "2.0", "method": "SimpleService.Event", "params": ["hello!"]}`
	resp := client.handleMessage(strings.NewReader(msg))
	if resp != nil {
		t.Errorf("No response expected for '%s', got: %+v", msg, resp)
	}

	if simpleService.lastEvent != "hello!" {
		t.Errorf("Event call should have modified the service data, service: %#v", simpleService)
	}

	// A named service event
	msg = `{"jsonrpc": "2.0", "method": "napre.Event", "params": {"number":1789, "name":"Bastille Day", "price":1.789, "flag":true}}`
	resp = client.handleMessage(strings.NewReader(msg))
	if resp != nil {
		t.Errorf("No response expected for '%s', got: %+v", msg, resp)
	}

	if namedService.lastEvent == nil {
		t.Errorf("Event call should have modified the service data, service: %+v", namedService)
	}

}

func TestNoServices(t *testing.T) {
	servs := []interface{}{}
	_, err := newWsJsonClient(nil, servs)
	if err == nil {
		t.Error("Client creation with empty services should have failed")
	}

	if !strings.Contains(err.Error(), "At least one service is required") {
		t.Error("Invalid error for empty services:", err)
	}
}

type EmptyService struct{}

type UnexpMethodService struct{}

func (*UnexpMethodService) Noop() {
}

func (*UnexpMethodService) notExported() {
}

func (*UnexpMethodService) WsMethods() map[string]string {
	return map[string]string{
		"noop":      "Noop",
		"fail_here": "notExported",
	}
}

type TooManyOutputsService struct{}

func (*TooManyOutputsService) ApiManyOutputs(n int) (int, string, error) {
	return 1, "Many", nil
}

type NoErrorOutputService struct{}

func (*NoErrorOutputService) ApiNoErr(n int) (int, string) {
	return 3, "no error"
}

func TestValidations(t *testing.T) {

	var unnamedService = struct {
	}{}
	var table = []struct {
		serv  interface{}
		error string
	}{
		{nil, "Attempt to add nil service instance"},
		{&EmptyService{}, "No exposed methods found"},
		{&unnamedService, "Unable to get a name for"},
		{&UnexpMethodService{}, "notExported is not a method of"},
		{&TooManyOutputsService{}, "Method 'ApiManyOutputs' must have 0 or 2 outputs"},
		{&NoErrorOutputService{}, "Method 'ApiNoErr' last output must be of type error"},
	}

	for _, row := range table {
		servs := []interface{}{row.serv}
		_, err := newWsJsonClient(nil, servs)
		if err == nil {
			t.Error("Client creation should have failed:", row.serv)
		}

		if !strings.Contains(err.Error(), row.error) {
			t.Errorf("Invalid error for service:%T, err:%v, expected:%s", row.serv, err, row.error)
		}

	}
}

func TestResults(t *testing.T) {
	client, _, _, _, err := createClient()
	if err != nil {
		t.Fatal(err)
	}

	var table = []struct {
		msg    string
		errMsg string
	}{
		{`{"jsonrpc":"2.0", "result":[1,2,3]}`, "Result with null id received"},
		{`{"jsonrpc":"2.0", "result":[1,2,3], "id": "yadayada"}`, "Result id must be an integer"},
		{`{"jsonrpc":"2.0", "result":[1,2,3], "id": 666}`, "No previous request found for result.id:666"},
	}

	for _, tc := range table {
		lc := startLogCapture()
		res := client.handleMessage(strings.NewReader(tc.msg))
		if res != nil {
			t.Error("Nil response expected", res)
		}
		lc.stop()
		if !lc.contains(tc.errMsg) {
			t.Errorf("Expected message not found in logs: msg: '%s', expected:'%s', logs: '%v'",
				tc.msg, tc.errMsg, lc.buffer)
		}
	}
}

func TestCalls(t *testing.T) {
	client, _, _, _, err := createClient()
	if err != nil {
		t.Fatal(err)
	}

	// output consumer goroutine
	outCh := make(chan *Request)
	go func() {
		for rqIf := range client.output {

			rq, ok := rqIf.(*Request)
			if !ok {
				t.Fatal("Received output is not a request", rqIf)
			}
			outCh <- rq
		}
	}()

	// Error when trying to send non JSON encodable parameters
	var params interface{}
	params = []complex128{1i, 3i, 5i}
	ch, err := client.CallMethod("someMethod", params)
	if ch != nil {
		t.Error("Non-nil channel returned", ch)
	}
	if err == nil {
		t.Error("An error was expected while trying to send complex numbers")
	}

	// Send a simple event
	params = &AllTypes{Number: 42, Name: "Meaning Of Life", Price: 42.42, Flag: true}
	err = client.SendEvent("someEvent", params)
	if err != nil {
		t.Error("Unexpected error while trying to send a simple event", err)
	}

	select {
	case resp := <-outCh:
		if resp.Id != nil {
			t.Errorf("Event should not have an id field: '%s'", resp)
		}

		expectedParams, err := json.Marshal(params)
		if err != nil {
			t.Fatal(err)
		}
		expected := &Request{
			Version: JSONRPCVersion,
			Method:  "someEvent",
			Id:      nil,
			Params:  expectedParams,
		}

		if !reflect.DeepEqual(resp, expected) {
			t.Errorf("Invalid output for simple event: %s", expected)
		}

	case <-time.After(200 * time.Millisecond):
		t.Fatal("No Output send for simple event")
	}

	// Method Call
	ch, err = client.CallMethod("someMethod", params)
	if err != nil {
		t.Fatal("Error calling method", err)
	}

	var resp *Request
	select {
	case resp = <-outCh:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("No Output send for method call")
	}

	if resp.Id == nil {
		t.Errorf("Method call should have an id field: '%s'", resp)
	}

	expectedParams, err := json.Marshal(params)
	if err != nil {
		t.Fatal(err)
	}
	expected := &Request{
		Version: JSONRPCVersion,
		Method:  "someMethod",
		Id:      resp.Id,
		Params:  expectedParams,
	}

	if !reflect.DeepEqual(resp, expected) {
		t.Errorf("Invalid output for method call: %s", expected)
	}

	if len(client.pendingResults) != 1 {
		t.Errorf("There should be 1 pending result: %+v", client.pendingResults)
	}

	//goroutine to consume results
	ch2 := make(chan json.RawMessage)
	ch2stop := make(chan bool)
	go func() {
		for result := range ch {
			ch2 <- result
		}
		ch2stop <- true
	}()

	//send response
	response := fmt.Sprintf(`{"jsonrpc":"2.0", "result":{"value1": "all_good"}, "id":%d}`, resp.Id)
	r := client.handleMessage(strings.NewReader(response))
	if r != nil {
		t.Fatalf("Result delivery should have no response, found: %s", r)
	}

	var rawResult json.RawMessage
	select {
	case rawResult = <-ch2:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Result not delivered on channel")
	}

	var result map[string]string
	err = json.Unmarshal(rawResult, &result)
	if err != nil {
		t.Error(err)
	}
	if result["value1"] != "all_good" {
		t.Errorf("Invalid result received: %+v", result)
	}

	select {
	case <-ch2stop:
	case <-time.After(200 * time.Millisecond):
		log.Fatal("Result channel wasn't closed")
	}

}
