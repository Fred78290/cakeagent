// Seamless window protocol, guest side. Mirror of Sources/cakedlib/Seamless/SeamlessProtocol.swift (version 5):
// every message is `u32 length` (type byte + payload) + `u8 type` + payload, little endian, strings are
// `u16 length` + UTF-8. Types 0x01..0x7F go guest -> host, 0x81..0xFF host -> guest.

#ifndef SEAMLESS_WIRE_H
#define SEAMLESS_WIRE_H

#include <stdbool.h>
#include <stddef.h>
#include <stdint.h>

#define SEAMLESS_VERSION 5
/* First version whose clipboard is a list of MIME typed items (G_CLIPBOARD_ITEMS / H_CLIPBOARD_ITEMS). Version 3 had
 * a text only message (0x09 / 0x8C), which this side no longer speaks: a link has one version, exactly. */
#define SEAMLESS_CLIPBOARD_ITEMS_VERSION 4
/* First version with H_KEYBOARD_LAYOUT: the host tells the guest the XKB layout of the keyboard that types into the windows. */
#define SEAMLESS_KEYBOARD_LAYOUT_VERSION 5
/* The longest XKB layout name of H_KEYBOARD_LAYOUT, in bytes (not counting the NUL). */
#define SEAMLESS_MAX_LAYOUT 32
#define SEAMLESS_DEFAULT_VSOCK_PORT 5100
#define SEAMLESS_MAX_STRING 4096
#define SEAMLESS_MAX_RECTS 1024
#define SEAMLESS_MAX_DIMENSION 16384
#define SEAMLESS_MAX_PIXELS 40000000
/* The data of all the items of a clipboard message, in bytes, in either direction. A bigger selection is not
 * transferred (an item that does not fit is left out). */
#define SEAMLESS_MAX_CLIPBOARD_TOTAL (32 * 1024 * 1024)
/* The items of a clipboard message, and the length of a MIME type. */
#define SEAMLESS_MAX_CLIPBOARD_ITEMS 16
#define SEAMLESS_MAX_MIME 127
/* Host messages are tiny, but for the clipboard; anything bigger is a broken or hostile peer. */
#define SEAMLESS_MAX_HOST_FRAME (SEAMLESS_MAX_CLIPBOARD_TOTAL + 64 * 1024)

enum seamless_guest_type {
	G_HELLO = 0x01,
	G_WINDOW_CREATE = 0x02,
	G_WINDOW_TITLE = 0x03,
	G_WINDOW_APPID = 0x04,
	G_WINDOW_HINTS = 0x05,
	G_WINDOW_FRAME = 0x06,
	G_WINDOW_DESTROY = 0x07,
	G_CURSOR = 0x08,
	/* 0x09 is the protocol 3 text clipboard, not spoken any more. */
	G_CLIPBOARD_ITEMS = 0x0A, /* u16 count, then per item: u16 mime length + MIME type + u32 length + bytes */
};

enum seamless_host_type {
	H_HELLO_ACK = 0x81,
	H_OUTPUT = 0x82,
	H_WINDOW_RESIZE = 0x83,
	H_WINDOW_CLOSE = 0x84,
	H_WINDOW_FOCUS = 0x85,
	H_POINTER_ENTER = 0x86,
	H_POINTER_LEAVE = 0x87,
	H_POINTER_MOTION = 0x88,
	H_POINTER_BUTTON = 0x89,
	H_POINTER_AXIS = 0x8A,
	H_KEY = 0x8B,
	/* 0x8C is the protocol 3 text clipboard, not spoken any more. */
	H_CLIPBOARD_ITEMS = 0x8D, /* same payload as G_CLIPBOARD_ITEMS */
	H_KEYBOARD_LAYOUT = 0x8E, /* string: XKB layout of the host's keyboard; empty: the layout the compositor started with */
};

enum seamless_window_kind { KIND_TOPLEVEL = 0, KIND_POPUP = 1 };

/* True for a MIME type the clipboard carries: type/subtype, printable ASCII without spaces, 3 to SEAMLESS_MAX_MIME
 * bytes. The X11 pseudo targets (TARGETS, UTF8_STRING...) have no slash and are not MIME types. */
bool seamless_valid_mime(const char *s, size_t n);

/* True for an XKB layout name H_KEYBOARD_LAYOUT carries: empty, or a lowercase letter then lowercase letters, digits and
 * underscores, SEAMLESS_MAX_LAYOUT bytes at most. */
bool seamless_valid_layout(const char *s, size_t n);

/* Walks the items of a validated clipboard payload (H_CLIPBOARD_ITEMS data). */
struct clip_iter {
	const uint8_t *p;
	size_t left;
	uint16_t remaining;
};

void clip_iter_init(struct clip_iter *it, const uint8_t *payload, size_t n);
/* The next item: its MIME type (NUL terminated, in mime[SEAMLESS_MAX_MIME + 1]) and data, which stays in the payload. */
bool clip_iter_next(struct clip_iter *it, char *mime, const uint8_t **data, size_t *len);

/* Growable output buffer. */
struct wbuf {
	uint8_t *p;
	size_t len, cap;
	bool failed;
};

void wbuf_free(struct wbuf *b);
void wbuf_u8(struct wbuf *b, uint8_t v);
void wbuf_u16(struct wbuf *b, uint16_t v);
void wbuf_u32(struct wbuf *b, uint32_t v);
void wbuf_i32(struct wbuf *b, int32_t v);
void wbuf_bytes(struct wbuf *b, const void *data, size_t n);
/* Truncated to SEAMLESS_MAX_STRING bytes without cutting a UTF-8 sequence. NULL is the empty string. */
void wbuf_str(struct wbuf *b, const char *s);

/* Starts a message (reserves the length) and finishes it by patching the length in. */
size_t wire_begin(struct wbuf *b, uint8_t type);
void wire_end(struct wbuf *b, size_t start);

/* H_POINTER_AXIS source (host_msg.u32a). */
enum axis_source {
	AXIS_CONTINUOUS = 0, /* trackpad: x/y in logical pixels */
	AXIS_WHEEL = 1,      /* mouse wheel: x/y in notches */
	AXIS_STOP = 2,       /* fingers lifted: x/y ignored */
};

/* A decoded host message. Unused fields are zero. */
struct host_msg {
	uint8_t type;
	uint32_t id;
	uint32_t u32a, u32b; /* helloAck/output: scale120 in u32a; resize: width, height; button: button; axis: enum axis_source in u32a */
	float x, y;          /* pointer position or axis deltas */
	bool flag;           /* focused / pressed */
	uint16_t version;
	uint32_t code; /* key */
	const uint8_t *data; /* clipboard: the validated payload, in the receive buffer: valid until the next rx_* call */
	size_t data_len;
	char layout[SEAMLESS_MAX_LAYOUT + 1]; /* keyboard layout: NUL terminated, validated */
};

/* 0 on success, -1 on a malformed message (the connection must be closed). */
int wire_parse_host(uint8_t type, const uint8_t *payload, size_t n, struct host_msg *out);

/* Incremental splitter for the host -> guest byte stream. */
struct rx {
	uint8_t *p;
	size_t len, cap;  /* bytes stored / allocated, starting at p */
	size_t head;      /* start of the unread data */
	size_t pending;   /* size of the message handed out by the last rx_next, dropped on the next call */
};

void rx_free(struct rx *r);
bool rx_append(struct rx *r, const uint8_t *data, size_t n);
/* 1: one message returned (payload valid until the next rx_* call), 0: need more bytes, -1: invalid stream. */
int rx_next(struct rx *r, uint8_t *type, const uint8_t **payload, size_t *n);

#endif
