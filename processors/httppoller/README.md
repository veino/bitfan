# HTTPPOLLER
HTTPPoller allows you to call an HTTP Endpoint, decode the output of it into an event

## Synopsys


|  SETTING  |  TYPE  | REQUIRED | DEFAULT VALUE |
|-----------|--------|----------|---------------|
| Add_field | hash   | false    | {}            |
| Tags      | array  | false    | []            |
| Type      | string | false    | ""            |
| codec     | codec  | false    | "plain"       |
| interval  | string | false    | ""            |
| method    | string | false    | "GET"         |
| url       | string | true     | ""            |
| target    | string | false    | ""            |


## Details

### Add_field
* Value type is hash
* Default value is `{}`

Add a field to an event

### Tags
* Value type is array
* Default value is `[]`

Add any number of arbitrary tags to your event.
This can help with processing later.

### Type
* Value type is string
* Default value is `""`

Add a type field to all events handled by this input

### codec
* Value type is codec
* Default value is `"plain"`

The codec used for input data. Input codecs are a convenient method for decoding
your data before it enters the input, without needing a separate filter in your bitfan pipeline

### interval
* Value type is string
* Default value is `""`

Use CRON or BITFAN notation

### method
* Value type is string
* Default value is `"GET"`

Http Method

### url
* This is a required setting.
* Value type is string
* Default value is `""`

URL

### target
* Value type is string
* Default value is `""`

When data is an array it stores the resulting data into the given target field.



## Configuration blueprint

```
httppoller{
	add_field => {}
	tags => []
	type => ""
	codec => "plain"
	interval => "every_10s"
	method => "GET"
	url=> "http://google.fr"
	target => ""
}
```