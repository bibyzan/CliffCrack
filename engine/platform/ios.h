// Shared between ios.m and window_ios.go (iOS only).
#pragma once

#include <stdint.h>

enum {
    CC_EVENT_TOUCH = 1, // id = the finger, code = input.TouchPhase, value/value2 = x/y in pixels
    CC_EVENT_RESIZE,
    CC_EVENT_FOCUS, // code = 1 active / 0 going to the background
};

typedef struct CCEvent {
    int      type;
    int      code;
    float    value, value2;
    uint64_t id;
} CCEvent;

#define CC_MAX_EVENTS 512

int   cc_take_events(CCEvent* out, int capacity);
void  cc_wait(int timeout_ms);  // sleeps until an event arrives or the time is up
void* cc_layer(void);           // the game view's CAMetalLayer
int   cc_active(void);          // 0 while the app is in the background: draw nothing
void  cc_size(int* width, int* height); // the drawable, in pixels
float cc_scale(void);           // pixels per point
int   cc_run(int argc, char** argv); // runs UIApplicationMain; never returns
