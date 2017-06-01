package wsjson

type MyApi struct {
	method1Calls int
}

type response struct {
	AnInt    int
	AnString string
	AnArray  []string
}

func (api *MyApi) PubMethod1(paramInt int, paramString string, paramArray []string) (response, error) {
	api.method1Calls++
	return response{
		AnInt:    paramInt,
		AnString: paramString,
		AnArray:  paramArray,
	}, nil
}

func (api *MyApi) NotPartOfApi() (int, error) {
	return 1, nil
}

func (api *MyApi) PubInvalid(param1 int) *response {
	return nil
}

/*
func TestGetMethod(t *testing.T) {
	am := newApiManager()
	_, _, err := am.getMethod("bla bla")
	if err == nil || err.Error() != "Invalid name: bla bla" {
		t.Errorf("Invalid error returned for invalid method name: %v", err)
	}

	_, _, err = am.getMethod("AAA.BBB")
	if err == nil || err.Error() != "API not found: AAA" {
		t.Errorf("Invalid error returned for non existent API; %v", err)
	}

	err = am.addObject("api1", "Pub", new(response))
	if err == nil || !strings.HasPrefix(err.Error(), "No api methods found for") {
		t.Errorf("Invalid error for no api methods: %v", err)
	}

	myApi := new(MyApi)

	err = am.addObject("api1", "Pub", myApi)
	if err != nil {
		t.Fatalf("Error while adding api: %v", err)
	}
	if am.numMethods() != 1 {
		t.Errorf("Only one method should have been loaded, methods: %d", am.numMethods())
	}

	_, _, err = am.getMethod("api1.Method1")
	if err != nil {
		t.Fatalf("Unexpected error while getting method: %v", err)
	}

	//log.Printf("method: %v", method)

}

func TestConvertParam(t *testing.T) {
	var f float64 = 22
	var i int = 1

	// simple integer conversion
	r, err := convertParam(f, reflect.TypeOf(i))
	if err != nil {
		t.Fatalf("Error converting float64 to int, %v", err)
	}

	if r.Type().Kind() != reflect.Int {
		t.Errorf("An int was expected: %v", r)
	}

	// array of strings
	arr := []interface{}{"one", "two"}
	var strArr []string
	narr, err := convertParam(arr, reflect.TypeOf(strArr))
	if err != nil {
		t.Fatalf("Error converting []interface{} to []string, %v", err)
	}

	if !reflect.DeepEqual(narr.Interface().([]string), []string{"one", "two"}) {
		t.Errorf("Invalid array returned: %#v", narr)
	}

	// []strint -> []int
	var intArr []int
	_, err = convertParam(arr, reflect.TypeOf(intArr))
	if err == nil {
		t.Fatalf("Convertion from string array to []int should have failed")
	}

	if !strings.Contains(err.Error(), "Incompatible types, expected: int, got: string") {
		t.Errorf("Invalid Error recived: %v", err)
	}
}

// Call registered method with json parsed parameters
func TestCallMethod(t *testing.T) {
	req := []byte(`[42, "Meaning of life", ["one", "two", "tree"]]`)
	var params []interface{}
	err := json.Unmarshal(req, &params)
	if err != nil {
		t.Fatalf("Error parsing json parameters: %v", err)
	}

	//fmt.Printf("params : %#v\n", params)
	am := newApiManager()
	am.addObject("api1", "Pub", new(MyApi))
	r, err := am.callMethod("api1.Method1", params)
	if err != nil {
		t.Fatalf("Error while calling method: %v", err)
	}

	resp, ok := r.(response)
	if !ok {
		t.Fatalf("Error casting response: %v", r)
	}

	if resp.AnInt != 42 || resp.AnString != "Meaning of life" {
		t.Fatalf("Invalid response: %#v\n", resp)
	}

}
*/
