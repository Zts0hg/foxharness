package schema

/*
	ContentPart preserves ordered tool content and host-owned artifact references.

Data is transient request or incoming image data and is never serialized into history.
URI describes an external resource link; it is not permission to read a local file.
*/
type ContentPart struct {
	Type        string `json:"type"`
	Text        string `json:"text,omitempty"`
	MIMEType    string `json:"mime_type,omitempty"`
	ArtifactKey string `json:"artifact_key,omitempty"`
	URI         string `json:"uri,omitempty"`
	Name        string `json:"name,omitempty"`
	Data        []byte `json:"-"`
}

/* CloneContentParts returns independently owned parts, including transient data. */
func CloneContentParts(parts []ContentPart) []ContentPart {
	if parts == nil {
		return nil
	}
	copy := append([]ContentPart{}, parts...)
	for i := range copy {
		copy[i].Data = append([]byte(nil), copy[i].Data...)
	}
	return copy
}

/* Clone returns a message whose mutable fields are independent of the source. */
func (m Message) Clone() Message {
	m.ContentParts = CloneContentParts(m.ContentParts)
	m.StructuredContent = append([]byte(nil), m.StructuredContent...)
	if m.ToolCalls != nil {
		m.ToolCalls = append([]ToolCall{}, m.ToolCalls...)
		for i := range m.ToolCalls {
			m.ToolCalls[i].Arguments = append([]byte(nil), m.ToolCalls[i].Arguments...)
		}
	}
	if m.Usage != nil {
		usage := *m.Usage
		m.Usage = &usage
	}
	return m
}

/* Clone returns an independently owned tool result for observers and adapters. */
func (r ToolResult) Clone() ToolResult {
	r.ContentParts = CloneContentParts(r.ContentParts)
	r.StructuredContent = append([]byte(nil), r.StructuredContent...)
	return r
}
