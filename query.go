package parseur

import (
	"sort"
)

type Query struct {
	query      string
	tags       *[]*Tag
	parser     *Parser
	subQueries *[]*Query
}

func (p *Parser) Query(query string) *Query {
	q := Query{query: query, parser: p}
	return &q
}

func (q *Query) Query(query string) *Query {
	newQ := Query{query: query, parser: q.parser, subQueries: nil}

	if q.subQueries == nil {
		q.subQueries = &[]*Query{}
	}

	list := append(*q.subQueries, &newQ)
	q.subQueries = &list

	return &newQ
}

func (q *Query) Last() *Tag {
	tags := q.GetTags()

	if tags == nil {
		return nil
	}

	length := len(*tags)

	if length == 0 {
		return nil
	}

	return (*tags)[length-1]
}

func (q *Query) First() *Tag {
	tags := q.GetTags()

	if tags == nil || len(*tags) == 0 {
		return nil
	}
	return (*tags)[0]
}

func (q *Query) Intersect(query *Query) *Query {
	queryIntersection := Query{
		parser: q.parser,
		query:  q.query + " + " + query.query,
		tags:   GetIntersection(q.GetTags(), query.GetTags()),
	}

	return &queryIntersection
}

func (q *Query) GetTags() *[]*Tag {
	if q.tags == nil {
		q.execute()
	}
	return q.tags
}

func (q *Query) execute() *Query {
	if q.tags != nil {
		return q
	}

	if q.parser == nil {
		return q
	}

	q.tags = q.parseQuery()

	return q
}

func (q *Query) parseQuery() *[]*Tag {
	var tags = q.tags
	var qualifiers *[]string
	length := len(q.query)

	for i := 0; i < length; i++ {

		switch q.query[i] {
		case ' ':
			for ; i < length && q.query[i] == ' '; i++ {
			}

			qualifiers, i = q.parseQualifiers(i)

			if len(*qualifiers) == 0 {
				continue
			}

			var filteredTags []*Tag

			for _, tag := range *tags {
				filterDeep(&tag.Children, qualifiers, &filteredTags)
			}

			tags = &filteredTags

			if tags == nil {
				return nil
			}

			continue

		case '>':

			if tags == nil {
				return nil
			}

			for i += 1; i < length && q.query[i] == ' '; i++ {
			}

			qualifiers, i = q.parseQualifiers(i)

			if qualifiers == nil {
				continue
			}

			var filteredTags []*Tag

			for _, t := range *tags {
				filteredTags = append(filteredTags, t.Children...)
			}

			*tags = (*tags)[:0]

			for _, s := range filteredTags {
				if matchQualifiersDeep(qualifiers, s) {
					*tags = append(*tags, s)
				}
			}

			if len(*tags) == 0 {
				return nil
			}

		case '*':
			tags = q.parser.GetTags("*")
		default:
			qualifiers, i = q.parseQualifiers(i)

			if qualifiers != nil {
				tags = q.extractQualifiers(qualifiers)
			}

			qualifiers = nil

		}
	}

	return tags
}

func filterDeep(tags *[]*Tag, qualifiers *[]string, container *[]*Tag) {
	for _, t := range *tags {
		if matchQualifiersDeep(qualifiers, t) {
			*container = append(*container, t)
		}

		filterDeep(&t.Children, qualifiers, container)
	}
}

func getQualifier(query *string, i int) int {
	length := len(*query)

	for ; length > i && isValidQualifierChar((*query)[i]); i++ {

	}

	return i
}

func isValidQualifierChar(c uint8) bool {
	return ('0' <= c && c <= '9') ||
		('A' <= c && c <= 'Z') ||
		('a' <= c && c <= 'z') ||
		c == '-' || c == '_'
}

func cmpQualifier(qualifiers *[]string) func(int, int) bool {
	return func(a, b int) bool {
		return (*qualifiers)[a][0] == '#'
	}
}

func (q *Query) parseQualifiers(i int) (*[]string, int) {
	var qualifiers []string

	length := len(q.query)

	for i < length {
		start := i

		if q.query[i] == '.' || q.query[i] == '#' {
			i += 1
		}

		i = getQualifier(&q.query, i)

		if i == start {
			break
		}

		qualifier := q.query[start:i]

		qualifiers = append(qualifiers, qualifier)

	}

	sort.Slice(qualifiers, cmpQualifier(&qualifiers))
	return &qualifiers, i - 1
}

func (q *Query) extractQualifiers(qualifiers *[]string) *[]*Tag {
	var tags *[]*Tag
	tags = q.parser.GetTags((*qualifiers)[0])

	if len(*qualifiers) == 1 {
		return tags
	}

	var filteredTags []*Tag

	*qualifiers = (*qualifiers)[1:]
	filterDeep(tags, qualifiers, &filteredTags)
	tags = &filteredTags

	if len(*tags) == 0 {
		return nil
	}

	return tags
}

func matchQualifiersDeep(qualifiers *[]string, t *Tag) bool {
	length := len(*qualifiers)

	for i := 0; i < length; i++ {
		qualifier := (*qualifiers)[i]

		if qualifier[0] == '.' {
			if !hasSubstr(t.Attributes["class"], qualifier[1:]) {
				return false
			}
		} else if qualifier[0] == '#' {
			if t.Attributes["id"] != qualifier[1:] {
				return false
			}
		} else if t.Name != qualifier {
			return false
		}
	}

	return true
}

func hasSubstr(hay string, needle string) bool {
	lengthNeedle := len(needle)
	length := len(hay)

	if lengthNeedle > length {
		return false
	}

	for i := 0; i < length; i++ {
		if hay[i] == needle[0] {
			if length-i < lengthNeedle {
				break
			}

			p := 0

			for z := i; p < lengthNeedle && needle[p] == hay[z]; {
				p++
				z++
			}

			stringMatched := p == length &&
				i+p >= length || !isValidQualifierChar(hay[i+p])
			if stringMatched {
				return true
			}
		}

		for i < length && hay[i] != ' ' {
			i++
		}

		for z := i + 1; z < length && hay[z] == ' '; {
			z++
			i++
		}
	}

	return false
}
