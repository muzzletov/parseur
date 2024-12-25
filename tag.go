package parseur

func (t *Tag) FindAll(name string) *[]*Tag {
	children := make([]*Tag, 0)

	for _, c := range t.Children {
		if c.Name == name {
			children = append(children, c)
		}

		children = append(children, *c.FindAll(name)...)
	}

	return &children
}

func (t *Tag) First(name string) *Tag {
	for _, c := range t.Children {
		if c.Name == name {
			return c
		}

		f := c.First(name)

		if f != nil {
			return f
		}
	}

	return nil
}

func (p *Parser) InnerText(t *Tag) string {
	return string((*p.body)[t.Body.Start:t.Body.End])
}
