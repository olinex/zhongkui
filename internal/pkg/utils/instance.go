package utils

import "reflect"

func NewInstance[T any]() T {
	var t T
	val := reflect.New(reflect.TypeOf(t)).Elem()
	return val.Interface().(T)
}
