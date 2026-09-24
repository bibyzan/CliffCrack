// Shared between android.c and window_android.go (Android only).
#pragma once

#include "android_native_app_glue.h"

enum {
    CC_EVENT_KEY = 1, // code = Android key code, value = 1 down / 0 up
    CC_EVENT_AXIS,    // code = input.PadAxis, value
    CC_EVENT_HAT,     // d-pad hat: value = x, value2 = y (-1, 0, 1)
    CC_EVENT_TOUCH,   // code = 1 touching / 0 lifted, value/value2 = x/y in pixels
    CC_EVENT_RESIZE,
    CC_EVENT_FOCUS, // code = 1 gained / 0 lost
};

typedef struct CCEvent {
    int   type;
    int   code;
    float value, value2;
} CCEvent;

#define CC_MAX_EVENTS 512

extern struct android_app* cc_app;
extern char                cc_shader_dir[512];

int   cc_take_events(CCEvent* out, int capacity);
int   cc_poll(int timeout_ms); // returns non-zero once the activity is being destroyed
float cc_density(void);
void  cc_finish(void);
