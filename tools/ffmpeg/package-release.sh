#!/usr/bin/env sh
set -eu

if [ "$#" -lt 1 ]; then
  echo "usage: $0 <platform> [sagaflow-binary] [ffmpeg-directory] [output-directory]" >&2
  exit 2
fi

platform="$1"
case "$platform" in
  windows-amd64|windows-arm64|linux-amd64|linux-arm64|darwin-amd64|darwin-arm64) ;;
  *) echo "unsupported platform: $platform" >&2; exit 2 ;;
esac

repo="$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)"
case "$platform" in
  windows-*) app_name="sagaflow.exe"; ffmpeg_name="ffmpeg.exe"; ffprobe_name="ffprobe.exe" ;;
  *) app_name="sagaflow"; ffmpeg_name="ffmpeg"; ffprobe_name="ffprobe" ;;
esac

app="${2:-$repo/bin/$app_name}"
tools="${3:-$repo/tools/ffmpeg/$platform}"
output="${4:-$repo/dist/sagaflow-$platform}"

for path in "$app" "$tools/$ffmpeg_name" "$tools/$ffprobe_name" "$tools/FFMPEG-LICENSE.txt"; do
  if [ ! -f "$path" ]; then
    echo "required release file is missing: $path" >&2
    exit 1
  fi
done

mkdir -p "$output"
install -m 0755 "$app" "$output/$app_name"
install -m 0755 "$tools/$ffmpeg_name" "$output/$ffmpeg_name"
install -m 0755 "$tools/$ffprobe_name" "$output/$ffprobe_name"
install -m 0644 "$tools/FFMPEG-LICENSE.txt" "$output/FFMPEG-LICENSE.txt"
install -m 0644 "$repo/LICENSE" "$output/SAGAFLOW-LICENSE.txt"
echo "prepared $platform release directory at $output"
