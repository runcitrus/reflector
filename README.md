# Reflector

Reflector is a code generator to make bindings between [gRPC](https://grpc.io) and [Wails](https://wails.io).
All gRPC methods will be available directly in the JS frontend.

## Chat Protocol

For example we use a project with name `chat`. Proto files for basic chat located in the `./protos/chat.proto`:

```proto
syntax = "proto3";
option go_package = "chat;pb";
package chat;

service Chat {
    rpc Subscribe (Empty) returns (stream Note);
    rpc Send (Note) returns (Empty);
}

message Empty {}

message Note {
    string name = 1;
    string message = 2;
}
```

Create directory `./backend/pb` and Generate gRPC code:

```
protoc \
    --go_out=./backend/pb \
    --go_opt=paths=source_relative \
    --go-grpc_out=./backend/pb \
    --go-grpc_opt=paths=source_relative \
    -I ./protos \
    ./protos/*.proto
```

## Generator

Generator is a basic command to start `reflector` package and generate bindings for Wails.

Create directory `./backend/reflector` to store generated bindings.
Create cmd to launch reflector for your gRPC code in `./cmd/reflector/main.go`:

```go
package main

import (
	"fmt"
	"os"

	"github.com/runcitrus/reflector"

	"chat/backend/pb"
)

func start() error {
	result, err := reflector.Generate(
		"chat/backend/pb",
		[]any{
			pb.NewChatClient,
		},
	)
	if err != nil {
		return err
	}

	goGenPath := "./backend/reflector/grpc.gen.go"
	goGen, err := os.OpenFile(
		goGenPath,
		os.O_CREATE|os.O_TRUNC|os.O_WRONLY,
		0644,
	)
	if err != nil {
		return fmt.Errorf("failed to open %s: %w", goGenPath, err)
	}
	defer goGen.Close()

	if _, err := goGen.Write(result); err != nil {
		return fmt.Errorf("failed to write to %s: %w", goGenPath, err)
	}

	return nil
}

func main() {
	if err := start(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
```

Now you can launch `go run ./cmd/reflector` to generate bindings for Wails.

## Bind to Wails

```go
import (
	"context"
	"strings"

	"github.com/wailsapp/wails/v2/pkg/application"
	"github.com/wailsapp/wails/v2/pkg/options"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"chat/backend/reflector"
)

func dial(ctx context.Context, target string) (*grpc.ClientConn, error) {
	if strings.IndexByte(target, ':') == -1 {
		target += ":8000"
	}

	return grpc.DialContext(
		ctx,
		target,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
}

func start() error {
	// ...

	app := application.NewWithOptions(&options.App{
		OnStartup: func(ctx context.Context) {
			reflector.Init(ctx, runtime.EventsEmit, dial)
		},
		Bind: any{
			&reflector.GRPC{},
			&reflector.ChatClient{},
		},
	})

	return app.Run()
}
```

- `dial` - this function will be use to start gRPC connection from JS
- `reflector.Init` - provide Wails context and functions to emit events and dial gRPC
- `&reflector.GRPC{}` - is a general methods: Connect and Disconnect
- `&reflector.ChatClient{}` - is a chat methods: Send and Subscribe

## Use gRPC in JS

```js
import { EventsOn } from '../wailsjs/runtime'
import { Connect, Disconnect } from "../wailsjs/go/reflector/GRPC.js"
import { Send, Subscribe } from "../wailsjs/go/reflector/ChatClient.js"

EventsOn('ChatClient_Subscribe', (addr, event) => {
    console.log(addr, event)
})

async function connect(address) {
    await Connect(address)
    await Subscribe(addr)
}

async function send(address, name, message) {
    await Send(address, { name, message })
}
```
