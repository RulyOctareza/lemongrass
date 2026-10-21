const SPS_NAL_TYPE = 7

function hex(byte: number): string {
  return byte.toString(16).padStart(2, '0')
}

// WebCodecs names an H.264 stream by the profile, constraint and level bytes of its SPS.
export function avcCodecString(annexB: Uint8Array): string | null {
  for (let i = 0; i + 6 < annexB.length; i++) {
    const startCode = annexB[i] === 0 && annexB[i + 1] === 0 && annexB[i + 2] === 1
    if (!startCode) continue
    const nal = i + 3
    if ((annexB[nal] & 0x1f) !== SPS_NAL_TYPE) continue
    return `avc1.${hex(annexB[nal + 1])}${hex(annexB[nal + 2])}${hex(annexB[nal + 3])}`
  }
  return null
}

export function concatBytes(a: Uint8Array, b: Uint8Array): Uint8Array {
  const out = new Uint8Array(a.length + b.length)
  out.set(a, 0)
  out.set(b, a.length)
  return out
}
