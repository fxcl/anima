GO_SRC = $(wildcard *.go) $(wildcard **/*.go)

define gen-badge
	curl \
		--data-urlencode "label=$(1)" \
		--data-urlencode "message=$(2)" \
		--data-urlencode "color=$(3)" \
		--output "$(4)" \
		https://img.shields.io/static/v1 
endef

define get-color
	if (( `echo "$(1) <= 50" | bc -l` )) ; then \
		echo red ; \
	elif (( $$(echo "$(1) > 80" | bc -l) )) ; then \
		echo green ; \
	else \
		echo orange ; \
	fi
endef

.PHONY: badges

badges: badges/coverage.svg badges/go-report-card.svg

coverage.out: ${GO_SRC}
	@go test -coverprofile=coverage.out -covermode=count  ./... ;

badges/coverage.svg: coverage.out
	@coverage=$$(go tool cover -func=coverage.out | grep total | grep -o '[0-9]\+\.[0-9]\+'); \
	if [ -z "$$coverage" ]; then \
		echo "ERROR: could not parse total coverage from coverage.out" >&2; \
		exit 1; \
	fi; \
	if (( $$(echo "$$coverage <= 50" | bc -l) )); then color=red; \
	elif (( $$(echo "$$coverage > 80" | bc -l) )); then color=green; \
	else color=orange; fi; \
	echo "coverage: $$coverage"; \
	curl --fail --data-urlencode "label=Coverage" \
		--data-urlencode "message=$$coverage%" \
		--data-urlencode "color=$$color" \
		--output "$@" \
		https://img.shields.io/static/v1

badges/go-report-card.svg: ${GO_SRC}
	$(eval grade = $(shell grade=$$(goreportcard-cli | head -1); echo $${grade:7}))
	$(eval grade_percent = $(shell echo '$(grade)' | grep -o "[0-9.]\+"))
	@echo 'go report card rating: $(grade)'
	$(eval label = Go Report)
	$(eval message = $(grade))
	$(eval color = $(shell $(call get-color,$(grade_percent))))
	$(call gen-badge,$(label),$(message),$(color),$@)
