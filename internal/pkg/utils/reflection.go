package utils

import "reflect"

func GetField[T any](obj any, name string) T {
	rv := reflect.ValueOf(obj)
	if rv.Kind() == reflect.Ptr {
		rv = rv.Elem()
	}
	field := rv.FieldByName(name)
	if !field.CanInterface() {
		field = reflect.NewAt(field.Type(), field.Addr().UnsafePointer()).Elem()
	}
	return field.Interface().(T)
}

func SetField(obj any, name string, value any) {
	rv := reflect.ValueOf(obj)
	if rv.Kind() == reflect.Ptr {
		rv = rv.Elem()
	}
	field := rv.FieldByName(name)
	if !field.CanSet() {
		field = reflect.NewAt(field.Type(), field.Addr().UnsafePointer()).Elem()
	}
	field.Set(reflect.ValueOf(value))
}
