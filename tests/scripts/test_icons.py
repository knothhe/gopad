"""Guard the production ICNS against macOS IconServices small-icon regressions."""

from pathlib import Path
import struct
import unittest


ROOT = Path(__file__).resolve().parents[2]


class ProductionIconTests(unittest.TestCase):
    def test_retina_pngs_replace_standard_resolution_small_slots(self):
        data = (ROOT / 'assets/gopad.icns').read_bytes()
        self.assertEqual(data[:4], b'icns')
        self.assertEqual(struct.unpack('>I', data[4:8])[0], len(data))
        expected = {
            b'ic11': 32, b'ic12': 64, b'ic07': 128, b'ic08': 256,
            b'ic13': 256, b'ic09': 512, b'ic14': 512, b'ic10': 1024,
        }
        seen = set()
        offset = 8
        while offset < len(data):
            self.assertGreaterEqual(len(data) - offset, 8, 'truncated ICNS header')
            kind, length = struct.unpack_from('>4sI', data, offset)
            self.assertGreaterEqual(length, 8)
            self.assertLessEqual(length, len(data) - offset)
            self.assertNotIn(
                kind, (b'icp4', b'icp5', b'ic04', b'ic05'),
                'standard-resolution small slots render incorrectly; run make icon',
            )
            if kind in expected:
                self.assertNotIn(kind, seen, 'duplicate icon representation')
                payload = data[offset + 8:offset + length]
                self.assertEqual(payload[:8], b'\x89PNG\r\n\x1a\n')
                self.assertEqual(payload[12:16], b'IHDR')
                self.assertEqual(
                    struct.unpack('>II', payload[16:24]),
                    (expected[kind], expected[kind]),
                )
                seen.add(kind)
            offset += length
        self.assertEqual(seen, set(expected), 'missing standard or Retina PNGs')


if __name__ == '__main__':
    unittest.main()
