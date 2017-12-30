package falconer

//go:generate gojson -input example.yaml -o config.go -fmt yaml -pkg falconer -name Config
//go:generate protoc --proto_path=vendor:. falconer.proto --go_out=plugins=grpc:.
