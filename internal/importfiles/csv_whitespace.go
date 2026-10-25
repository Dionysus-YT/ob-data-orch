package importfiles

import "bytes"

// csvWhitespaceGuard 只检查固定 CSV 解析器会丢失的未引用首尾空白，不解析、转换或输出数据值。
// 与摘要读取共享流，跨块只保留识别语法所需的尾部字节；工作缓冲区在完成或失败时清零。
// state 的 0/1/2 分别表示未引用字段、引用字段内部、引用字段闭合后的间隔。
type csvWhitespaceGuard struct {
	separator, quote, escape  []byte
	pending                   []byte
	state                     int
	atStart, trailing         bool
	row                       int
	skipHeader, loss, invalid bool
	bomChecked                bool
}

func newCSVWhitespaceGuard(cSeparator, cQuote, cEscape string, skipHeader bool) *csvWhitespaceGuard {
	if cSeparator == "" {
		cSeparator = ","
	}
	if cQuote == "" {
		cQuote = "\""
	}
	return &csvWhitespaceGuard{separator: []byte(cSeparator), quote: []byte(cQuote), escape: []byte(cEscape), atStart: true, skipHeader: skipHeader}
}

func (g *csvWhitespaceGuard) markLoss() {
	if !g.skipHeader || g.row > 0 {
		g.loss = true
	}
}

func (g *csvWhitespaceGuard) endField() {
	if g.state == 0 && g.trailing {
		g.markLoss()
	}
	g.state, g.atStart, g.trailing = 0, true, false
}

func (g *csvWhitespaceGuard) plain(value byte) {
	// CharUtils.isWhitespace 的固定位图只包含 TAB、LF、VT、FF、CR 和空格；换行在外层处理。
	space := value == ' ' || value == '\t' || value == '\v' || value == '\f'
	if g.atStart && space {
		g.markLoss()
	}
	g.trailing, g.atStart = space, false
}

func (g *csvWhitespaceGuard) feed(data []byte, final bool) {
	g.pending = append(g.pending, data...)
	index := 0
	lookahead := max(3, len(g.separator), len(g.quote)*2, len(g.escape)+1)
	for index < len(g.pending) {
		remaining := g.pending[index:]
		if !final && len(remaining) < lookahead {
			break
		}
		if !g.bomChecked {
			g.bomChecked = true
			if bytes.HasPrefix(remaining, []byte{0xef, 0xbb, 0xbf}) {
				index += 3
				continue
			}
		}
		if len(g.escape) > 0 && !bytes.Equal(g.escape, g.quote) && bytes.HasPrefix(remaining, g.escape) && g.state != 2 {
			if len(remaining) <= len(g.escape) {
				g.invalid = true
				break
			}
			if g.state == 0 {
				g.plain(remaining[len(g.escape)])
			}
			index += len(g.escape) + 1
			continue
		}
		if bytes.HasPrefix(remaining, g.quote) {
			if g.state == 1 {
				if bytes.HasPrefix(remaining[len(g.quote):], g.quote) {
					index += len(g.quote) * 2
					continue
				}
				g.state = 2
				index += len(g.quote)
				continue
			}
			if g.state == 0 && g.atStart {
				g.state, g.atStart = 1, false
				index += len(g.quote)
				continue
			}
		}
		if g.state != 1 && bytes.HasPrefix(remaining, g.separator) {
			g.endField()
			index += len(g.separator)
			continue
		}
		value := remaining[0]
		if g.state != 1 && (value == '\n' || value == '\r') {
			g.endField()
			g.row++
			if value == '\r' && len(remaining) > 1 && remaining[1] == '\n' {
				index++
			}
			index++
			continue
		}
		if g.state == 0 {
			g.plain(value)
		}
		if g.state == 2 && value != ' ' && value != '\t' && value != '\v' && value != '\f' {
			g.invalid = true
		}
		index++
	}
	left := copy(g.pending, g.pending[index:])
	for i := left; i < len(g.pending); i++ {
		g.pending[i] = 0
	}
	g.pending = g.pending[:left]
	if final {
		if g.state == 1 {
			g.invalid = true
		}
		g.endField()
		for i := range g.pending {
			g.pending[i] = 0
		}
		g.pending = nil
	}
}

func (g *csvWhitespaceGuard) clear() {
	for i := range g.pending {
		g.pending[i] = 0
	}
	g.pending = nil
}
