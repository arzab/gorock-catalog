package rockhttp

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/arzab/gorock-kit/rockhandlers"
)

const defaultMaxMemory = 32 << 20 // multipart parts kept in memory

var (
	fileHeaderType  = reflect.TypeFor[*multipart.FileHeader]()
	fileHeadersType = reflect.TypeFor[[]*multipart.FileHeader]()
	durationType    = reflect.TypeFor[time.Duration]()
)

// bind fills the struct behind in from the request:
//
//   - `path:"id"`     path value of the pattern "/orders/{id}"
//   - `query:"page"`  query parameter (repeat it for a slice field)
//   - `header:"X-Y"`  request header
//   - `form:"name"`   form / multipart field; *multipart.FileHeader or
//     []*multipart.FileHeader for uploaded files
//   - JSON body       decoded into the whole struct when Content-Type is JSON
//
// Tags inside embedded structs are bound too (e.g. an embedded BearerAuth
// with `header:"Authorization"`).
//
// Order: JSON body, then form, query, header, path (later wins).
func bind(r *http.Request, in any, maxBody int64) error {
	v := reflect.ValueOf(in)

	ct, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
	switch {
	case ct == "application/json" && r.Body != nil && r.Body != http.NoBody:
		if err := decodeJSON(r, in, maxBody); err != nil {
			return err
		}
	case ct == "multipart/form-data":
		r.Body = http.MaxBytesReader(nil, r.Body, maxBody)
		if err := r.ParseMultipartForm(defaultMaxMemory); err != nil {
			return rockhandlers.Invalid("invalid multipart form").Wrap(err)
		}
	case ct == "application/x-www-form-urlencoded":
		r.Body = http.MaxBytesReader(nil, r.Body, maxBody)
		if err := r.ParseForm(); err != nil {
			return rockhandlers.Invalid("invalid form").Wrap(err)
		}
	}

	return bindFields(r, v.Elem())
}

func bindFields(r *http.Request, s reflect.Value) error {
	t := s.Type()
	for i := 0; i < t.NumField(); i++ {
		sf := t.Field(i)
		f := s.Field(i)
		if sf.Anonymous {
			if f.Kind() == reflect.Pointer && f.Type().Elem().Kind() == reflect.Struct && sf.IsExported() {
				if f.IsNil() {
					f.Set(reflect.New(f.Type().Elem()))
				}
				f = f.Elem()
			}
			if f.Kind() == reflect.Struct && f.CanSet() {
				if err := bindFields(r, f); err != nil {
					return err
				}
				continue
			}
		}
		if !sf.IsExported() {
			continue
		}
		if name := sf.Tag.Get("form"); name != "" {
			if err := bindForm(r, f, name); err != nil {
				return err
			}
		}
		if name := sf.Tag.Get("query"); name != "" {
			if vals, ok := r.URL.Query()[name]; ok {
				if err := setValues(f, vals); err != nil {
					return paramError("query", name, err)
				}
			}
		}
		if name := sf.Tag.Get("header"); name != "" {
			if vals := r.Header.Values(name); len(vals) > 0 {
				if err := setValues(f, vals); err != nil {
					return paramError("header", name, err)
				}
			}
		}
		if name := sf.Tag.Get("path"); name != "" {
			if val := r.PathValue(name); val != "" {
				if err := setValues(f, []string{val}); err != nil {
					return paramError("path", name, err)
				}
			}
		}
	}
	return nil
}

func decodeJSON(r *http.Request, in any, maxBody int64) error {
	dec := json.NewDecoder(http.MaxBytesReader(nil, r.Body, maxBody))
	if err := dec.Decode(in); err != nil && !errors.Is(err, io.EOF) {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			return rockhandlers.Invalid("request body is larger than %d bytes", tooBig.Limit)
		}
		var typeErr *json.UnmarshalTypeError
		if errors.As(err, &typeErr) && typeErr.Field != "" {
			return rockhandlers.Invalid("invalid JSON body").
				WithFields(map[string]string{typeErr.Field: "must be " + jsonKind(typeErr.Type)}).Wrap(err)
		}
		// The decoder's text names Go types: it stays in the cause, for logs.
		return rockhandlers.Invalid("invalid JSON body").Wrap(err)
	}
	return nil
}

// jsonKind names a Go type the way a JSON client thinks of it.
func jsonKind(t reflect.Type) string {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	switch t.Kind() {
	case reflect.Bool:
		return "a boolean"
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64:
		return "a number"
	case reflect.String:
		return "a string"
	case reflect.Slice, reflect.Array:
		return "an array"
	default:
		return "an object"
	}
}

// errUnsupported marks a Params field type the binder cannot fill: a bug in
// the Params, not in the request.
var errUnsupported = errors.New("rockhttp: unsupported field type")

// paramError reports a bad query, header, path or form value as Invalid with
// the field in Fields, or an unsupported field type as Internal.
func paramError(source, name string, err error) error {
	if errors.Is(err, errUnsupported) {
		return &rockhandlers.Error{Kind: rockhandlers.KindInternal, Err: fmt.Errorf("%s %q: %w", source, name, err)}
	}
	return rockhandlers.Invalid("invalid %s", source).WithFields(map[string]string{name: err.Error()})
}

func bindForm(r *http.Request, f reflect.Value, name string) error {
	switch f.Type() {
	case fileHeaderType, fileHeadersType:
		if r.MultipartForm == nil {
			return nil
		}
		files := r.MultipartForm.File[name]
		if len(files) == 0 {
			return nil
		}
		if f.Type() == fileHeaderType {
			f.Set(reflect.ValueOf(files[0]))
		} else {
			f.Set(reflect.ValueOf(files))
		}
		return nil
	}
	if r.Form == nil {
		return nil
	}
	if vals, ok := r.Form[name]; ok {
		if err := setValues(f, vals); err != nil {
			return paramError("form", name, err)
		}
	}
	return nil
}

// setValues converts string values to the field's type: strings, bools,
// ints, uints, floats, time.Duration, pointers to those, and slices of them.
func setValues(f reflect.Value, vals []string) error {
	if f.Kind() == reflect.Slice && f.Type() != fileHeadersType {
		s := reflect.MakeSlice(f.Type(), len(vals), len(vals))
		for i, v := range vals {
			if err := setString(s.Index(i), v); err != nil {
				return err
			}
		}
		f.Set(s)
		return nil
	}
	return setString(f, vals[0])
}

func setString(f reflect.Value, v string) error {
	if f.Kind() == reflect.Pointer {
		p := reflect.New(f.Type().Elem())
		if err := setString(p.Elem(), v); err != nil {
			return err
		}
		f.Set(p)
		return nil
	}
	if f.Type() == durationType {
		d, err := time.ParseDuration(v)
		if err != nil {
			return errors.New("must be a duration such as 5s")
		}
		f.SetInt(int64(d))
		return nil
	}
	switch f.Kind() {
	case reflect.String:
		f.SetString(v)
	case reflect.Bool:
		b, err := strconv.ParseBool(v)
		if err != nil {
			return errors.New("must be true or false")
		}
		f.SetBool(b)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		n, err := strconv.ParseInt(strings.TrimSpace(v), 10, f.Type().Bits())
		if err != nil {
			return errors.New("must be an integer")
		}
		f.SetInt(n)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		n, err := strconv.ParseUint(strings.TrimSpace(v), 10, f.Type().Bits())
		if err != nil {
			return errors.New("must be a non-negative integer")
		}
		f.SetUint(n)
	case reflect.Float32, reflect.Float64:
		n, err := strconv.ParseFloat(strings.TrimSpace(v), f.Type().Bits())
		if err != nil {
			return errors.New("must be a number")
		}
		f.SetFloat(n)
	default:
		return fmt.Errorf("%w %s", errUnsupported, f.Type())
	}
	return nil
}
