PYTHON ?= python3
GO = $(PYTHON) scripts/go.py
CHANNEL ?= dev
IDENTITY ?=
SIGNING_P12 ?=
UPDATER_KEY ?=
SIGNING_FLAGS ?=
INSTALL_DIR ?= $(HOME)/Applications/gopad Dev.app
VERSION_BUMP ?= patch
RELEASE_FLAGS ?=
RELEASE_REPO ?=
RELEASE_TAG ?=

.PHONY: run preview build package install check test test-scripts test-frontend icon clean signing-list signing-config signing-upload release release-initial release-notes

run:
	$(GO) run .

preview:
	$(GO) run . --preview --listen 127.0.0.1:5189

build:
	$(PYTHON) scripts/macos.py build --channel "$(CHANNEL)"

package:
	$(PYTHON) scripts/macos.py package --channel "$(CHANNEL)"

install:
	$(PYTHON) scripts/macos.py install --channel "$(CHANNEL)" --destination "$(INSTALL_DIR)"

signing-list:
	$(PYTHON) scripts/macos.py signing-list

signing-config:
	$(PYTHON) scripts/macos.py signing-config --identity "$(IDENTITY)"

# Missing file arguments are collected by the interactive signing setup.
signing-upload:
	@$(PYTHON) scripts/signing_upload.py --p12 "$(SIGNING_P12)" --updater-key "$(UPDATER_KEY)" --repo "$(RELEASE_REPO)" $(SIGNING_FLAGS)

test:
	$(GO) test -race ./...

test-frontend:
	npm test --prefix tests/frontend

test-scripts:
	PYTHONDONTWRITEBYTECODE=1 $(PYTHON) -m unittest discover -s tests/scripts -v

check: test test-frontend test-scripts
	$(GO) vet ./...

icon:
	./scripts/icon.sh

# Both previews call the same release engine; neither builds or uploads locally.
release-initial:
	$(PYTHON) scripts/release.py initial $(RELEASE_FLAGS)

release:
	$(PYTHON) scripts/release.py $(VERSION_BUMP) $(RELEASE_FLAGS)

release-notes:
	$(PYTHON) scripts/release-notes.py "$(RELEASE_REPO)" "$(RELEASE_TAG)"

clean:
	rm -rf build
