# fetch, build and install a dependency
# Just include this file and provide URL variable with the source location

TGZ ?= $(notdir $(URL))
DIR ?= $(shell a=$(TGZ); echo $${a%.t*z})
DEST = $(CURDIR)

all: build

build: $(DIR)/.build_ok

clean:
	rm -rf $(DIR)

$(DIR)/.build_ok: $(DIR)
	cd $(DIR) && ./configure --prefix=$(DEST) && make && make install
	touch $(DIR)/.build_ok

$(DIR): $(TGZ)
	tar xvfz $(TGZ) || { rm -rf $(DIR); false; }
	
$(TGZ):
	curl -L -o $@.tmp $(URL)	
	mv $@.tmp $@
