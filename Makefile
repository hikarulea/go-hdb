# builds and tests project via go tools
all:
	@echo "update dependencies"
	go get -u ./...
	go mod tidy
	@echo "build and test"
	go build -v ./...
	go vet ./...
	golint -set_exit_status=true ./...
	staticcheck -checks all -fail none ./...
	golangci-lint run ./...
	@echo execute tests on latest go version	
	go test ./...
	@echo execute tests on older supported go versions
	go1.20.14 test ./...
	@echo execute tests on future supported go versions
	go1.22rc2 test ./...
	gotip test ./...

#see fsfe reuse tool (https://git.fsfe.org/reuse/tool)
	@echo "reuse (license) check"
	pipx run reuse lint

#go generate
generate:
	@echo "generate"
	go generate ./...

#install additional tools
tools:
#install stringer
	@echo "install latest stringer version"
	go install golang.org/x/tools/cmd/stringer@latest
#install linter
	@echo "install latest go linter version"
	go install golang.org/x/lint/golint@latest
#install staticcheck
	@echo "install latest staticcheck version"
	go install honnef.co/go/tools/cmd/staticcheck@latest
#install golangci-lint
	@echo "install latest golangci-lint version"
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

#install additional go versions
go:
	go install golang.org/dl/go1.20.14@latest
	go1.20.14 download
	go install golang.org/dl/go1.22rc2@latest
	go1.22rc2 download
	go install golang.org/dl/gotip@latest
	gotip download

