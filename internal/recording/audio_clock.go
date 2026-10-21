package recording

// QPC anchors the stream to video once. Device frame positions then preserve
// each sample exactly, without independently rounding every packet's QPC.
type packetClock struct {
	anchored     bool
	position     uint64
	lastPosition uint64
	start, next  int64
}

func (c *packetClock) place(position uint64, observed int64, frames uint32, valid bool) int64 {
	start := c.next
	if valid {
		if !c.anchored || position < c.lastPosition {
			c.position, c.start, c.anchored = position, observed, true
		}
		start = c.start + int64(position-c.position)
		c.lastPosition = position
	} else if !c.anchored && c.next == 0 {
		start = observed
	}
	c.next = start + int64(frames)
	return start
}
