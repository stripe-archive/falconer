package falconer

//go:generate gojson -input example.yaml -o config.go -fmt yaml -pkg falconer -name Config
//go:generate protoc -I. -Ivendor falconer.proto --gofast_out=plugins=grpc:.
