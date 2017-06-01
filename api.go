package wsjson

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"sync"
)

const (
	// Default prefix for exposed methods of registered API's
	defMethodPrefix = "Api"
)

var (
	typeOfError = reflect.TypeOf((*error)(nil)).Elem()
)

type NameProvider interface {
	WsName() string
}

// API's must implement this interface to use a different prefix for exposed API methods
type PrefixProvider interface {
	WsPrefix() string
}

// API's must implement this interface to explicitely define the exposed methods
type MethodsProvider interface {
	WsMethods() map[string]string
}

// A single API
type apiObject struct {
	name    string
	apiType reflect.Type
	value   reflect.Value
	methods map[string]*apiMethod
}

type apiMethod struct {
	object     *apiObject
	method     reflect.Method
	argTypes   []reflect.Type
	isEvent    bool
	returnType reflect.Type
}

/*
type methodCache struct {
	methods map[reflect.Method]*apiMethod
	mutex   sync.RWMutex
}
*/

type apiManager struct {
	objects map[string]*apiObject
	mutex   sync.RWMutex
}

func newApiManager() *apiManager {
	m := &apiManager{
		objects: make(map[string]*apiObject),
	}
	return m
}

// Return the number of objects registered
func (m *apiManager) numObjects() int {
	return len(m.objects)
}

// Return the total number of methods registered
func (m *apiManager) numMethods() int {
	var c int
	for _, o := range m.objects {
		c += len(o.methods)
	}
	return c
}

// Register an Object to serve requests
// the resulting API methods will have "name." as prefix
func (m *apiManager) addObject(object interface{}) error {
	//log.Printf("Adding Object: %#v", object)
	api := &apiObject{
		apiType: reflect.TypeOf(object),
		value:   reflect.ValueOf(object),
		methods: make(map[string]*apiMethod),
	}

	nameProv, ok := object.(NameProvider)
	if ok {
		api.name = nameProv.WsName()
	}

	if api.name == "" {
		api.name = reflect.Indirect(api.value).Type().Name()
	}

	if api.name == "" {
		return fmt.Errorf("Unable to get a name for: %T", object)
	}

	type nameMeth struct {
		name   string
		method reflect.Method
	}

	// channel to receive generated the methods
	methodsChan := make(chan *nameMeth)
	methProv, ok := object.(MethodsProvider)
	if ok {
		// List of methods is provided explicitely
		go func() {
			for name, methodName := range methProv.WsMethods() {

				method, ok := api.apiType.MethodByName(methodName)
				if !ok {
					panic(fmt.Sprintf("WSMethods(): %s is not a method of %v",
						methodName, api.apiType))
				}
				methodsChan <- &nameMeth{name, method}
			}
			close(methodsChan)
		}()
	} else {
		// Get Methods by prefix
		prefix := defMethodPrefix
		prefProv, ok := object.(PrefixProvider)
		if ok {
			prefix = prefProv.WsPrefix()
		}

		go func() {
			// log.Printf("Number of methods: %v", api.apiType.NumMethod())
			for i := 0; i < api.apiType.NumMethod(); i++ {
				method := api.apiType.Method(i)
				if strings.HasPrefix(method.Name, prefix) {
					methodsChan <- &nameMeth{strings.TrimPrefix(method.Name, prefix), method}
				}
			}
			close(methodsChan)
		}()
	}

	for nm := range methodsChan {
		//log.Printf("Checking method: %#v", nm)
		publicName := nm.name
		method := nm.method
		methodType := method.Type
		// method must be exported
		if method.PkgPath != "" {
			return fmt.Errorf("Method must be exported: %v", method)
		}

		// must have at least one input argument
		if methodType.NumIn() < 1 {
			return fmt.Errorf("Method must have at least one input argument: %v", method)
		}

		// First parameter must have the type of the registered object
		//firstType := methodType.In(0)

		// methods must have 2 outputs, events have none
		numOut := methodType.NumOut()
		var isEvent bool
		if numOut == 0 {
			isEvent = true
		} else if numOut == 2 {
			isEvent = false
		} else {
			return fmt.Errorf("Method %v must have 1 or 2 outputs, found: %d", method, numOut)
		}

		var returnType reflect.Type
		if !isEvent {
			returnType = methodType.Out(0)
			// Method last output must be an error
			if errType := methodType.Out(numOut - 1); errType != typeOfError {
				return fmt.Errorf("Last output must be of type error, method: %q", method)
			}
		}

		am := &apiMethod{
			object:     api,
			method:     method,
			isEvent:    isEvent,
			argTypes:   make([]reflect.Type, methodType.NumIn()-1),
			returnType: returnType,
		}
		for j := 1; j < methodType.NumIn(); j++ {
			am.argTypes[j-1] = methodType.In(j)
		}
		api.methods[publicName] = am
	}
	if len(api.methods) == 0 {
		return fmt.Errorf("No api methods found for %#v", object)
	}

	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.objects[api.name] = api
	return nil
}

// Get a method by name using notation <API>.<MethodName>
func (m *apiManager) getMethod(name string) (*apiMethod, error) {
	nameParts := strings.Split(name, ".")
	if len(nameParts) != 2 {
		return nil, NewError(ErrorMethodNotFound, "Invalid method name: %s", name)
	}

	apiName := nameParts[0]
	methodName := nameParts[1]

	m.mutex.RLock()
	api, ok := m.objects[apiName]
	m.mutex.RUnlock()
	if !ok {
		return nil, NewError(ErrorMethodNotFound, "API not found: %s", apiName)
	}

	method, ok := api.methods[methodName]
	if !ok {
		return nil, NewError(ErrorMethodNotFound, "API %s doesn't have the %s method", apiName, methodName)
	}

	return method, nil
}

// Call a registered API method
func (m *apiManager) callMethod(name string, params json.RawMessage) (interface{}, error) {
	method, err := m.getMethod(name)
	if err != nil {
		return nil, err
	}

	paramValues, err := method.decodeParams(params)
	if err != nil {
		return nil, err
	}
	response := method.method.Func.Call(paramValues)

	if method.isEvent {
		//Events have no return values
		return nil, nil
	}

	if len(response) != 2 {
		return nil, fmt.Errorf("Response should had 2 values, got: %#v", response)
	}

	if response[1].IsNil() {
		err = nil
	} else {
		e, ok := response[1].Interface().(error)
		if !ok {
			return nil, fmt.Errorf("Last parameter should have been an error, got: %#v, %t", response, response[1].Type().Kind() == reflect.Interface)
		}
		err = e
	}

	return response[0].Interface(), err
}

// Decode json params according to the method signature using reflection
func (am *apiMethod) decodeParams(params json.RawMessage) ([]reflect.Value, error) {
	typesLen := len(am.argTypes)
	paramValues := make([]reflect.Value, typesLen+1)
	paramValues[0] = am.object.value

	// If method has only one parameter and it is an struct then params must be send as an object
	if typesLen == 1 {
		firstType := am.argTypes[0]
		isPtr := false
		if firstType.Kind() == reflect.Ptr {
			firstType = firstType.Elem()
			isPtr = true
		}
		if firstType.Kind() == reflect.Struct {
			value := reflect.New(firstType)
			err := json.Unmarshal(params, value.Interface())
			if err != nil {
				//log.Printf("Error decoding parameters, params: %q, type: %#v, %v", params, firstType.Name(), err)
				return nil, NewError(ErrorInvalidParams, "Params must be an object")
			}
			if !isPtr {
				value = value.Elem()
			}

			paramValues[1] = value
			return paramValues, nil
		}
	}

	var paramsArray []json.RawMessage
	if len(params) > 0 { // params may be ommited
		err := json.Unmarshal(params, &paramsArray)
		if err != nil {
			return nil, NewError(ErrorInvalidParams, "Params must be an array, %v, %v", err, params)
		}
	}

	if len(paramsArray) != typesLen {
		return nil, NewError(
			ErrorInvalidParams,
			"Wrong number of arguments, expected: %d, got: %d",
			typesLen,
			len(paramsArray),
		)
	}

	for i, par := range paramsArray {
		value := reflect.New(am.argTypes[i])
		err := json.Unmarshal(par, value.Interface())
		if err != nil {
			return nil, NewError(
				ErrorInvalidParams, "Unable to decode parameter %d: %v",
				i, err.Error(),
			)
		}
		paramValues[i+1] = value.Elem()
	}

	return paramValues, nil

}
