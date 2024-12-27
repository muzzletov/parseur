package parseur

type Tag struct {
	Name       string
	Namespace  string
	Children   []*Tag
	Attributes map[string]string
	Body       Offset
	Tag        Offset
}
