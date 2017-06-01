package wsjson

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"sync"
)

const (
	// Default prefix for exposed methods of registered services
	defMethodPrefix = "Api"
)

var (
	typeOfError = reflect.TypeOf((*error)(nil)).Elem()
)

type NameProvider interface {
	WsName() string
}

// Services must implement this interface to use a different prefix for exposed methods
type PrefixProvider interface {
	WsPrefix() string
}

// Services must implement this interface to explicitely define the exposed methods
type MethodsProvider interface {
	WsMethods() map[string]string
}

// A single service
type service struct {
	instance interface{}
	name     string
	servType reflect.Type
	value    reflect.Value
	methods  map[string]*serviceMethod
}

// An exposed service method
type serviceMethod struct {
	service    *service
	method     *reflect.Method
	argTypes   []reflect.Type
	isEvent    bool
	returnType reflect.Type
}

type serviceManager struct {
	services map[string]*service
	mutex    sync.RWMutex
}

// Add all methods form the instance whose name starts with a prefix
func (serv *service) addMethodsByPrefix() error {
	prefix := defMethodPrefix
	prefProv, ok := serv.instance.(PrefixProvider)
	if ok {
		prefix = prefProv.WsPrefix()
	}

	for i := 0; i < serv.servType.NumMethod(); i++ {
		method := serv.servType.Method(i)
		if !strings.HasPrefix(method.Name, prefix) {
			continue
		}

		servMethod, err := newServiceMethod(serv, &method)
		if err != nil {
			return err
		}
		name := strings.TrimPrefix(method.Name, prefix)
		serv.methods[name] = servMethod
	}
	return nil
}

// Add methods using a provider
func (serv *service) addMethodsFromProvider(provider MethodsProvider) error {
	for name, methodName := range provider.WsMethods() {
		method, ok := serv.servType.MethodByName(methodName)
		if !ok {
			return fmt.Errorf("WSMethods(): %s is not a method of %v", methodName, serv.servType)
		}

		servMethod, err := newServiceMethod(serv, &method)
		if err != nil {
			return err
		}
		serv.methods[name] = servMethod
	}
	return nil
}

// New serviceMethod instance
func newServiceMethod(serv *service, method *reflect.Method) (*serviceMethod, error) {
	methodType := method.Type

	// method must be exported
	if method.PkgPath != "" {
		return nil, fmt.Errorf("Method must be exported: %v", method)
	}

	// must have at least one input argument
	if methodType.NumIn() < 1 {
		return nil, fmt.Errorf("Method must have at least one input argument: %v", method)
	}

	// methods must have 2 outputs, events have none
	numOut := methodType.NumOut()
	var isEvent bool
	if numOut == 0 {
		isEvent = true
	} else if numOut == 2 {
		isEvent = false
	} else {
		return nil, fmt.Errorf("Method '%s' must have 0 or 2 outputs, found: %d", method.Name, numOut)
	}

	var returnType reflect.Type
	if !isEvent {
		returnType = methodType.Out(0)
		// Method last output must be an error
		if errType := methodType.Out(numOut - 1); errType != typeOfError {
			return nil, fmt.Errorf("Method '%s' last output must be of type error", method.Name)
		}
	}

	sm := &serviceMethod{
		service:    serv,
		method:     method,
		isEvent:    isEvent,
		argTypes:   make([]reflect.Type, methodType.NumIn()-1),
		returnType: returnType,
	}
	for j := 1; j < methodType.NumIn(); j++ {
		sm.argTypes[j-1] = methodType.In(j)
	}
	return sm, nil
}

// Decode json params according to the method signature using reflection
func (am *serviceMethod) decodeParams(params json.RawMessage) ([]reflect.Value, error) {
	typesLen := len(am.argTypes)
	paramValues := make([]reflect.Value, typesLen+1)
	paramValues[0] = am.service.value

	// If method has only one parameter and it is an struct then params must be send as an service
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

func newServiceManager() *serviceManager {
	m := &serviceManager{
		services: make(map[string]*service),
	}
	return m
}

// Return the number of services registered
func (m *serviceManager) numServices() int {
	return len(m.services)
}

// Return the total number of methods registered
func (m *serviceManager) numMethods() int {
	var c int
	for _, o := range m.services {
		c += len(o.methods)
	}
	return c
}

// Register an Service to serve requests
// the resulting service methods will have "name." as prefix
func (m *serviceManager) addService(instance interface{}) error {
	if instance == nil {
		return fmt.Errorf("Attempt to add nil service instance")
	}

	//log.Printf("Adding Service: %#v", instance)
	serv := &service{
		instance: instance,
		servType: reflect.TypeOf(instance),
		value:    reflect.ValueOf(instance),
		methods:  make(map[string]*serviceMethod),
	}

	nameProv, ok := instance.(NameProvider)
	if ok {
		serv.name = nameProv.WsName()
	}

	if serv.name == "" {
		serv.name = reflect.Indirect(serv.value).Type().Name()
	}

	if serv.name == "" {
		return fmt.Errorf("Unable to get a name for: %T", instance)
	}

	methProv, ok := instance.(MethodsProvider)
	var err error
	if ok {
		err = serv.addMethodsFromProvider(methProv)
	} else {
		err = serv.addMethodsByPrefix()
	}

	if err != nil {
		return err
	}

	if len(serv.methods) == 0 {
		return fmt.Errorf("No exposed methods found for %#v", instance)
	}

	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.services[serv.name] = serv
	return nil
}

// Get a method by name using notation <ServiceName>.<MethodName>
func (m *serviceManager) getMethod(name string) (*serviceMethod, error) {
	nameParts := strings.Split(name, ".")
	if len(nameParts) != 2 {
		return nil, NewError(ErrorMethodNotFound, "Invalid method name: %s", name)
	}

	servName := nameParts[0]
	methodName := nameParts[1]

	m.mutex.RLock()
	api, ok := m.services[servName]
	m.mutex.RUnlock()
	if !ok {
		return nil, NewError(ErrorMethodNotFound, "API not found: %s", servName)
	}

	method, ok := api.methods[methodName]
	if !ok {
		return nil, NewError(ErrorMethodNotFound, "API %s doesn't have the %s method", servName, methodName)
	}

	return method, nil
}

// Call an exposed service method
func (m *serviceManager) callMethod(name string, params json.RawMessage) (interface{}, error) {
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
