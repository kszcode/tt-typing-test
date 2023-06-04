package main

func generateTestFromData(data []byte, raw bool, split bool) func() []segment {
	if raw {
		return func() []segment { return []segment{segment{string(data), "", -4}} }
	} else if split {
		paragraphs := getParagraphs(string(data))
		i := 0

		return func() []segment {
			if i < len(paragraphs) {
				p := paragraphs[i]
				i++
				return []segment{segment{p, "", -3}}
			} else {
				return nil
			}
		}
	} else {
		return func() []segment {
			var segments []segment

			for _, p := range getParagraphs(string(data)) {
				segments = append(segments, segment{p, "", -2})
			}

			return segments
		}
	}
}
