APP_NAME   := TDAManager
APP_ID     := devsecops.tda.manager
OUTPUT_DIR := dist
PKG        := ./cmd/flow-launcher

LDFLAGS := -s -w

.PHONY: all build-linux build-windows fyne-cross-windows clean

all: build-linux build-windows

build-linux:
	@mkdir -p $(OUTPUT_DIR)
	CGO_ENABLED=1 GOOS=linux GOARCH=amd64 \
	  go build -trimpath \
	  -ldflags "$(LDFLAGS)" \
	  -o $(OUTPUT_DIR)/$(APP_NAME) $(PKG)

build-windows:
	@mkdir -p $(OUTPUT_DIR)
	CGO_ENABLED=1 GOOS=windows GOARCH=amd64 \
	  CC=x86_64-w64-mingw32-gcc \
	  go build -trimpath \
	  -ldflags "$(LDFLAGS) -H=windowsgui" \
	  -o $(OUTPUT_DIR)/$(APP_NAME).exe $(PKG)

# Usa fyne-cross (imagen Docker con toolchain completo de MinGW + Fyne).
# Requiere Docker corriendo.
fyne-cross-windows:
	fyne-cross windows \
	  -arch amd64 \
	  -app-id $(APP_ID) \
	  -output $(APP_NAME).exe \
	  $(PKG)

clean:
	rm -rf $(OUTPUT_DIR) fyne-cross/dist
