#!/bin/sh
# Run the Swift tests with coverage and fail below the line-coverage floor
# for one target's sources.
set -eu

cd "$(dirname "$0")/.."

package=macos
target=PRNannyNative
tests=PRNannyNativeTests
floor=82

swift test --package-path "$package" --enable-code-coverage -Xswiftc -warnings-as-errors
codecov=$(dirname "$(swift test --package-path "$package" --show-codecov-path)")
bin=$(dirname "$codecov")
# The native build system puts every test target in one PackageTests bundle;
# swiftbuild makes one bundle per test target.
bundle=
for name in "$tests" PRNannyPackageTests; do
	if [ -f "$bin/$name.xctest/Contents/MacOS/$name" ]; then
		bundle="$bin/$name.xctest/Contents/MacOS/$name"
		break
	fi
done
if [ -z "$bundle" ]; then
	printf "No test bundle for %s under %s.\n" "$tests" "$bin" >&2
	exit 1
fi
mkdir -p coverage
xcrun llvm-cov report "$bundle" \
	-instr-profile "$codecov/default.profdata" \
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
