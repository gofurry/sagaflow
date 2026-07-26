export function binarizeMaskAlpha(source: Uint8ClampedArray) {
  const output = new Uint8ClampedArray(source.length)
  for (let index = 0; index < source.length; index += 4) {
    const value = source[index + 3] >= 128 ? 255 : 0
    output[index] = value
    output[index + 1] = value
    output[index + 2] = value
    output[index + 3] = 255
  }
  return output
}
