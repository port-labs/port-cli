package skills

import (
	"bytes"
	"fmt"
	"unicode/utf16"
	"unicode/utf8"
)

var (
	utf8BOM    = []byte{0xef, 0xbb, 0xbf}
	utf16LEBOM = []byte{0xff, 0xfe}
	utf16BEBOM = []byte{0xfe, 0xff}
)

func decodeSkillFileContent(path string, content []byte) (string, error) {
	switch {
	case bytes.HasPrefix(content, utf8BOM):
		content = content[len(utf8BOM):]
	case bytes.HasPrefix(content, utf16LEBOM):
		return decodeUTF16SkillFile(path, content[len(utf16LEBOM):], true)
	case bytes.HasPrefix(content, utf16BEBOM):
		return decodeUTF16SkillFile(path, content[len(utf16BEBOM):], false)
	}
	if !utf8.Valid(content) {
		return "", fmt.Errorf("%s: invalid UTF-8", path)
	}
	return string(content), nil
}

func decodeUTF16SkillFile(path string, content []byte, littleEndian bool) (string, error) {
	if len(content)%2 != 0 {
		return "", fmt.Errorf("%s: invalid UTF-16 content", path)
	}
	runes := make([]uint16, 0, len(content)/2)
	for i := 0; i < len(content); i += 2 {
		if littleEndian {
			runes = append(runes, uint16(content[i])|uint16(content[i+1])<<8)
		} else {
			runes = append(runes, uint16(content[i])<<8|uint16(content[i+1]))
		}
	}
	if !validUTF16(runes) {
		return "", fmt.Errorf("%s: invalid UTF-16 content", path)
	}
	return string(utf16.Decode(runes)), nil
}

func validUTF16(runes []uint16) bool {
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		switch {
		case r >= 0xd800 && r <= 0xdbff:
			if i+1 >= len(runes) || runes[i+1] < 0xdc00 || runes[i+1] > 0xdfff {
				return false
			}
			i++
		case r >= 0xdc00 && r <= 0xdfff:
			return false
		}
	}
	return true
}
