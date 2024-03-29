package reflector

import (
	"bytes"
	"context"
	"embed"
	"fmt"
	"html/template"
	"reflect"

	"google.golang.org/grpc"
)

type reflector struct {
	goBuf bytes.Buffer
}

var (
	//go:embed templates/*
	_fs        embed.FS
	_templates = template.Must(template.ParseFS(_fs, "templates/*.tmpl"))
)

func Generate(pkg string, items []any) ([]byte, error) {
	r := &reflector{}

	type tmplHead struct {
		Package string
	}

	err := _templates.ExecuteTemplate(
		&r.goBuf,
		"head",
		&tmplHead{
			Package: pkg,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to execute template: %w", err)
	}

	for _, item := range items {
		if err := r.bindGRPC(item); err != nil {
			return nil, err
		}
	}

	return r.goBuf.Bytes(), nil
}

// Unary RPCs where the client sends a single request to the server
// and gets a single response back, just like a normal function call.
func (r *reflector) grpc_bindUnary(
	objectName string,
	method reflect.Method, // gRPC method
) error {
	inType := method.Type.In(1)
	if inType.Kind() == reflect.Ptr {
		inType = inType.Elem()
	}
	numFields := countPublicFields(inType)

	type tmplDataUnary struct {
		ObjectName  string
		MethodName  string
		RequestType string
		HasRequest  bool
	}

	const templateName = "func-unary"
	err := _templates.ExecuteTemplate(
		&r.goBuf,
		templateName,
		&tmplDataUnary{
			ObjectName:  objectName,
			MethodName:  method.Name,
			RequestType: inType.Name(),
			HasRequest:  numFields > 0,
		},
	)

	if err != nil {
		return fmt.Errorf("failed to execute template %s: %w", templateName, err)
	}

	return nil
}

// Server streaming RPCs where the client sends a request to the server
// and gets a stream to read a sequence of messages back.
func (r *reflector) grpc_bindServerStreaming(
	objectName string,
	method reflect.Method, // gRPC method
) error {
	inType := method.Type.In(1)
	if inType.Kind() == reflect.Ptr {
		inType = inType.Elem()
	}
	numFields := countPublicFields(inType)

	type tmplDataServerStreaming struct {
		ObjectName   string
		MethodName   string
		RequestType  string
		HasRequest   bool
		ResponseType string
	}

	const templateName = "func-server-streaming"
	err := _templates.ExecuteTemplate(
		&r.goBuf,
		templateName,
		&tmplDataServerStreaming{
			ObjectName:   objectName,
			MethodName:   method.Name,
			RequestType:  inType.Name(),
			HasRequest:   numFields > 0,
			ResponseType: method.Type.Out(0).Name(),
		},
	)
	if err != nil {
		return fmt.Errorf("failed to execute template %s: %w", templateName, err)
	}

	return nil
}

func (r *reflector) bindGRPC(fn any) error {
	fnVal := reflect.ValueOf(fn)
	fnType := fnVal.Type()

	if fnType.Kind() != reflect.Func {
		return fmt.Errorf("bindGRPC: fn must be a function")
	}

	clientType := fnType.Out(0)

	if clientType.Kind() != reflect.Interface {
		return fmt.Errorf("bindGRPC: fn must return an interface to gRPC client")
	}

	objectName := clientType.Name()

	type tmplDataService struct {
		ObjectName string
	}

	err := _templates.ExecuteTemplate(
		&r.goBuf,
		"service",
		&tmplDataService{
			ObjectName: objectName,
		},
	)
	if err != nil {
		return fmt.Errorf("failed to execute template service: %w", err)
	}

	for i := 0; i < clientType.NumMethod(); i++ {
		method := clientType.Method(i)

		// Check if the method has the correct arguments
		if method.Type.NumIn() != 3 {
			continue
		}

		// first argument must be a context.Context
		if method.Type.In(0) != reflect.TypeOf((*context.Context)(nil)).Elem() {
			continue
		}

		inType := method.Type.In(1)
		inElem := inType.Elem()

		// second argument must be a pointer to struct
		if inType.Kind() != reflect.Ptr || inElem.Kind() != reflect.Struct {
			continue
		}

		// Check if the method has the correct results
		if method.Type.NumOut() != 2 {
			continue
		}

		// second result must be an error
		if method.Type.Out(1) != reflect.TypeOf((*error)(nil)).Elem() {
			continue
		}

		// check kind of the service method
		outType := method.Type.Out(0)

		if outType.Kind() == reflect.Ptr && outType.Elem().Kind() == reflect.Struct {
			if err = r.grpc_bindUnary(objectName, method); err != nil {
				return fmt.Errorf(
					"bind unary method for object %s: %w",
					objectName,
					err,
				)
			}
			continue
		}

		clientStreamInterface := reflect.TypeOf((*grpc.ClientStream)(nil)).Elem()
		if outType.Implements(clientStreamInterface) {
			if err = r.grpc_bindServerStreaming(objectName, method); err != nil {
				return fmt.Errorf(
					"bind server streaming method for object %s: %w",
					objectName,
					err,
				)
			}
			continue
		}
	}

	return nil
}

func countPublicFields(in reflect.Type) int {
	count := 0
	for i := 0; i < in.NumField(); i++ {
		field := in.Field(i)
		if field.PkgPath == "" { // PkgPath is empty for public fields
			count += 1
		}
	}
	return count
}
