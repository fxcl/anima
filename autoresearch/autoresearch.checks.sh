#!/bin/bash
# Behavior-alignment gate: the java_align / contract-gap / error-code guards must stay green.
set -uo pipefail
cd "$(dirname "$0")/.."
go test -count=1 -run 'TestAlign|TestGap|TestErrorCode|TestAtomicNilDB' . 2>&1 | tail -3
