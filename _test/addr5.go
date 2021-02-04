package main

import (
	"encoding/json"
	"fmt"
	"net/url"
)

func main() {
	body := []byte(`{
		"BODY_1": "VALUE_1",
		"BODY_2": "VALUE_2",
		"BODY_3": null,
		"BODY_4": {
			"BODY_1": "VALUE_1",
			"BODY_2": "VALUE_2",
			"BODY_3": null
		},
		"BODY_5": [
			"VALUE_1",
			"VALUE_2",
			"VALUE_3"
		]
	}`)

	values := url.Values{}

	var rawData map[string]interface{}
	err := json.Unmarshal(body, &rawData)
	if err != nil {
		fmt.Println("can't parse body")
		return
	}

	for key, val := range rawData {
		switch val.(type) {
		case string, bool, float64:
			values.Add(key, fmt.Sprint(val))
		case nil:
			values.Add(key, "")
		case map[string]interface{}, []interface{}:
			jsonVal, err := json.Marshal(val)
			if err != nil {
				fmt.Println("can't encode json")
				return
			}
			values.Add(key, string(jsonVal))
		}
	}
	fmt.Println(values.Get("BODY_1"))
	fmt.Println(values.Get("BODY_2"))
	fmt.Println(values.Get("BODY_3"))
	fmt.Println(values.Get("BODY_4"))
	fmt.Println(values.Get("BODY_5"))
}

// Output:
// VALUE_1
// VALUE_2
// 
// {"BODY_1":"VALUE_1","BODY_2":"VALUE_2","BODY_3":null}
// ["VALUE_1","VALUE_2","VALUE_3"]
