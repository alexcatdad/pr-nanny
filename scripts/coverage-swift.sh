#!/bin/sh
# Run the Swift tests with coverage and fail below the line-coverage floor
# for one target's sources.
set -eu

cd "$(dirname "$0")/.."

package=macos
target=PRNannyNative
tests=PRNannyNativeTests
floor=69

swift test --package-path "$package" --enable-code-coverage -Xswiftc -warnings-as-errors
bin=$(swift build --package-path "$package" --show-bin-path)
mkdir -p coverage
xcrun llvm-cov report "$bin/$tests.xctest/Contents/MacOS/$tests" \
	-instr-profile "$bin/codecov/default.profdata" \
	"$PWD/$package/Sources/$target" > coverage/swift-summary.txt

# Line coverage is the tenth column of the TOTAL row.
total=$(awk '/^TOTAL/ { gsub(/%/, "", $10); print $10 }' coverage/swift-summary.txt)
if [ -z "$total" ]; then
	printf '%s\n' 'Could not read total Swift coverage.' >&2
	exit 1
fi
if ! awk -v total="$total" -v floor="$floor" 'BEGIN { exit !(total >= floor) }'; then
	cat coverage/swift-summary.txt >&2
	printf '%s line coverage %s%% is below the %s%% floor.\n' "$target" "$total" "$floor" >&2
	exit 1
fi
printf '%s line coverage: %s%% (%s%% floor)\n' "$target" "$total" "$floor"
