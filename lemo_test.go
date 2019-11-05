/**
* @program: lemo
*
* @description:
*
* @author: lemo
*
* @create: 2019-10-05 14:19
**/

package lemo

import (
	"reflect"
	"testing"

	"github.com/mitchellh/mapstructure"
)

type User struct {
	Name string `json:"name" mapstructure:"name"`
	Addr string `json:"addr" mapstructure:"addr"`
	Age  int    `json:"age" mapstructure:"age"`
}

var user User

func StructToMap(input interface{}) map[string]interface{} {
	var output = make(map[string]interface{})

	var kf = reflect.TypeOf(input)

	if kf.Kind() == reflect.Ptr {
		return nil
	}

	if kf.Kind() != reflect.Struct {
		return nil
	}

	var vf = reflect.ValueOf(input)

	for i := 0; i < kf.NumField(); i++ {
		output[kf.Field(i).Tag.Get("json")] = vf.Field(i).Interface()
	}

	return output
}

var m = map[string]interface{}{
	"name": "xixi",
	"addr": "haha",
	"age":  11,
}

func BenchmarkParseMessage(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = mapstructure.WeakDecode(m, &user)
	}
}
