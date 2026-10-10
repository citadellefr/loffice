import 'dart:typed_data';

/// Compresses data as zlib does (RFC 1950, 1951), in plain Dart: the same
/// on every platform, a browser included. Matches are searched in the last
/// 32 KB and written with the fixed codes: a little larger than what zlib
/// makes, for a fraction of its code.
Uint8List deflate(Uint8List data) {
  final out = _Bits()
    ..byte(0x78)
    ..byte(0x9C)
    ..bits(1, 1) // the only block
    ..bits(1, 2); // of fixed codes

  final head = Int32List(_hashSize)..fillRange(0, _hashSize, -1);
  final previous = Int32List(_window);
  final n = data.length;
  var i = 0;
  while (i < n) {
    var best = 0, distance = 0;
    if (i + _minMatch <= n) {
      final h = _hash(data, i);
      var candidate = head[h];
      final limit = i + _maxMatch < n ? _maxMatch : n - i;
      for (var chain = 0; candidate >= 0 && i - candidate <= _window && chain < _maxChain; chain++) {
        if (data[candidate + best] == data[i + best]) {
          var length = 0;
          while (length < limit && data[candidate + length] == data[i + length]) {
            length++;
          }
          if (length > best) {
            best = length;
            distance = i - candidate;
            if (length == limit) break;
          }
        }
        final next = previous[candidate & (_window - 1)];
        if (next >= candidate) break;
        candidate = next;
      }
      previous[i & (_window - 1)] = head[h];
      head[h] = i;
    }
    if (best < _minMatch) {
      out.literal(data[i]);
      i++;
      continue;
    }
    out.match(best, distance);
    // what the match covers can be matched in turn
    for (var j = i + 1; j < i + best && j + _minMatch <= n; j++) {
      final h = _hash(data, j);
      previous[j & (_window - 1)] = head[h];
      head[h] = j;
    }
    i += best;
  }
  out.symbol(256);
  out.flush();

  var a = 1, b = 0;
  for (var k = 0; k < n; k++) {
    a += data[k];
    if (a >= 65521) a -= 65521;
    b += a;
    if (b >= 65521) b -= 65521;
  }
  out
    ..byte(b >> 8)
    ..byte(b & 0xFF)
    ..byte(a >> 8)
    ..byte(a & 0xFF);
  return out.bytes();
}

const _window = 1 << 15;
const _hashSize = 1 << 15;
const _minMatch = 3;
const _maxMatch = 258;
const _maxChain = 32;

int _hash(Uint8List d, int i) => ((d[i] << 10) ^ (d[i + 1] << 5) ^ d[i + 2]) & (_hashSize - 1);

/// The lengths and distances each code starts at; a code carries as many
/// more bits as its range needs.
const _lengths = [3, 4, 5, 6, 7, 8, 9, 10, 11, 13, 15, 17, 19, 23, 27, 31, 35, 43, 51, 59, 67, 83, 99, 115, 131, 163, 195, 227, 258];
const _lengthBits = [0, 0, 0, 0, 0, 0, 0, 0, 1, 1, 1, 1, 2, 2, 2, 2, 3, 3, 3, 3, 4, 4, 4, 4, 5, 5, 5, 5, 0];
const _distances = [1, 2, 3, 4, 5, 7, 9, 13, 17, 25, 33, 49, 65, 97, 129, 193, 257, 385, 513, 769, 1025, 1537, 2049, 3073, 4097, 6145, 8193, 12289, 16385, 24577];
const _distanceBits = [0, 0, 0, 0, 1, 1, 2, 2, 3, 3, 4, 4, 5, 5, 6, 6, 7, 7, 8, 8, 9, 9, 10, 10, 11, 11, 12, 12, 13, 13];

class _Bits {
  final _out = BytesBuilder(copy: false);
  final _chunk = Uint8List(1 << 16);
  var _used = 0;
  var _hold = 0;
  var _count = 0;

  void byte(int b) {
    if (_used == _chunk.length) {
      _out.add(Uint8List.fromList(_chunk));
      _used = 0;
    }
    _chunk[_used++] = b;
  }

  /// Writes the low [count] bits of [value], the lowest first.
  void bits(int value, int count) {
    _hold |= value << _count;
    _count += count;
    while (_count >= 8) {
      byte(_hold & 0xFF);
      _hold >>= 8;
      _count -= 8;
    }
  }

  /// Writes a code, whose bits go the highest first.
  void code(int value, int count) {
    var reversed = 0;
    for (var i = 0; i < count; i++) {
      reversed = (reversed << 1) | ((value >> i) & 1);
    }
    bits(reversed, count);
  }

  /// A literal, a length code or the end of the block, in the fixed codes.
  void symbol(int s) {
    if (s < 144) {
      code(0x30 + s, 8);
    } else if (s < 256) {
      code(0x190 + s - 144, 9);
    } else if (s < 280) {
      code(s - 256, 7);
    } else {
      code(0xC0 + s - 280, 8);
    }
  }

  void literal(int b) => symbol(b);

  void match(int length, int distance) {
    var l = _lengths.length - 1;
    while (_lengths[l] > length) {
      l--;
    }
    symbol(257 + l);
    bits(length - _lengths[l], _lengthBits[l]);
    var d = _distances.length - 1;
    while (_distances[d] > distance) {
      d--;
    }
    code(d, 5);
    bits(distance - _distances[d], _distanceBits[d]);
  }

  void flush() {
    if (_count > 0) byte(_hold & 0xFF);
    _hold = 0;
    _count = 0;
  }

  Uint8List bytes() {
    _out.add(Uint8List.sublistView(_chunk, 0, _used));
    return _out.takeBytes();
  }
}
