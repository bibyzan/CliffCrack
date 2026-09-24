//go:build android

// NativeActivity side of the Android platform layer (C). The glue runs
// android_main on its own thread; everything here runs on that thread, which
// is also where Go runs the game loop.
#include "android.h"

#include <android/asset_manager.h>
#include <android/log.h>
#include <android/window.h>
#include <errno.h>
#include <jni.h>
#include <pthread.h>
#include <stdio.h>
#include <string.h>
#include <sys/stat.h>
#include <unistd.h>

#include "_cgo_export.h"

struct android_app* cc_app;
char                cc_shader_dir[512];

static CCEvent cc_events[CC_MAX_EVENTS];
static int     cc_event_count;

static void push(int type, int code, float value, float value2) {
    if (cc_event_count < CC_MAX_EVENTS) {
        cc_events[cc_event_count++] = (CCEvent){type, code, value, value2};
    }
}

int cc_take_events(CCEvent* out, int capacity) {
    int n = cc_event_count < capacity ? cc_event_count : capacity;
    memcpy(out, cc_events, sizeof(CCEvent) * (size_t)n);
    cc_event_count = 0;
    return n;
}

// ---- stdout/stderr -> logcat -------------------------------------------------

static int logfd[2];

static void* log_thread(void* arg) {
    (void)arg;
    char    buf[1024];
    ssize_t n;
    while ((n = read(logfd[0], buf, sizeof buf - 1)) > 0) {
        if (buf[n - 1] == '\n') --n;
        buf[n] = 0;
        __android_log_write(ANDROID_LOG_INFO, "CliffCrack", buf);
    }
    return NULL;
}

static void start_logger(void) {
    setvbuf(stdout, NULL, _IOLBF, 0);
    setvbuf(stderr, NULL, _IONBF, 0);
    if (pipe(logfd) != 0) return;
    dup2(logfd[1], STDOUT_FILENO);
    dup2(logfd[1], STDERR_FILENO);
    pthread_t t;
    if (pthread_create(&t, NULL, log_thread, NULL) == 0) pthread_detach(t);
}

// ---- assets ------------------------------------------------------------------

// Copies assets/shaders/* out of the APK into internal storage, because the
// renderer loads shaders from files.
static void extract_shaders(void) {
    snprintf(cc_shader_dir, sizeof cc_shader_dir, "%s/shaders", cc_app->activity->internalDataPath);
    mkdir(cc_app->activity->internalDataPath, 0700);
    mkdir(cc_shader_dir, 0700);
    AAssetManager* mgr = cc_app->activity->assetManager;
    AAssetDir*     dir = AAssetManager_openDir(mgr, "shaders");
    const char*    name;
    int            count = 0;
    while ((name = AAssetDir_getNextFileName(dir)) != NULL) {
        char src[256], dst[768];
        snprintf(src, sizeof src, "shaders/%s", name);
        snprintf(dst, sizeof dst, "%s/%s", cc_shader_dir, name);
        AAsset* asset = AAssetManager_open(mgr, src, AASSET_MODE_BUFFER);
        if (!asset) continue;
        FILE* f = fopen(dst, "wb");
        if (f) {
            fwrite(AAsset_getBuffer(asset), 1, (size_t)AAsset_getLength(asset), f);
            fclose(f);
            ++count;
        } else {
            __android_log_print(ANDROID_LOG_ERROR, "CliffCrack", "can't write %s: %s", dst, strerror(errno));
        }
        AAsset_close(asset);
    }
    AAssetDir_close(dir);
    __android_log_print(ANDROID_LOG_INFO, "CliffCrack", "extracted %d shaders to %s", count, cc_shader_dir);
}

// ---- window ------------------------------------------------------------------

// Hides the status and navigation bars (sticky immersive mode) and keeps the
// screen on. NativeActivity has no Java of our own, so this goes through JNI.
static void go_fullscreen(void) {
    ANativeActivity_setWindowFlags(cc_app->activity, AWINDOW_FLAG_KEEP_SCREEN_ON | AWINDOW_FLAG_FULLSCREEN, 0);

    JavaVM* vm = cc_app->activity->vm;
    JNIEnv* env = NULL;
    if ((*vm)->AttachCurrentThread(vm, &env, NULL) != JNI_OK) return;
    jobject   activity = cc_app->activity->clazz;
    jclass    activity_class = (*env)->GetObjectClass(env, activity);
    jmethodID get_window = (*env)->GetMethodID(env, activity_class, "getWindow", "()Landroid/view/Window;");
    jobject   window = (*env)->CallObjectMethod(env, activity, get_window);
    jclass    window_class = (*env)->GetObjectClass(env, window);
    jmethodID get_decor = (*env)->GetMethodID(env, window_class, "getDecorView", "()Landroid/view/View;");
    jobject   decor = (*env)->CallObjectMethod(env, window, get_decor);
    jclass    view_class = (*env)->GetObjectClass(env, decor);
    jmethodID set_flags = (*env)->GetMethodID(env, view_class, "setSystemUiVisibility", "(I)V");
    // IMMERSIVE_STICKY | FULLSCREEN | HIDE_NAVIGATION | LAYOUT_* | LAYOUT_STABLE
    const int flags = 0x1000 | 0x4 | 0x2 | 0x400 | 0x200 | 0x100;
    (*env)->CallVoidMethod(env, decor, set_flags, flags);
    if ((*env)->ExceptionCheck(env)) (*env)->ExceptionClear(env);
    (*vm)->DetachCurrentThread(vm);
}

static void on_app_cmd(struct android_app* app, int32_t cmd) {
    switch (cmd) {
    case APP_CMD_INIT_WINDOW:
        go_fullscreen();
        ccNativeWindow(app->window);
        break;
    case APP_CMD_TERM_WINDOW:
        // The window dies once this returns: the renderer must drop its
        // surface now, not on the next frame.
        ccNativeWindow(NULL);
        break;
    case APP_CMD_WINDOW_RESIZED:
    case APP_CMD_CONFIG_CHANGED:
        push(CC_EVENT_RESIZE, 0, 0, 0);
        break;
    case APP_CMD_GAINED_FOCUS:
        push(CC_EVENT_FOCUS, 1, 0, 0);
        go_fullscreen(); // the bars come back after dialogs and app switches
        break;
    case APP_CMD_LOST_FOCUS:
        push(CC_EVENT_FOCUS, 0, 0, 0);
        break;
    }
}

// ---- input -------------------------------------------------------------------

static float max_axis(const AInputEvent* e, int32_t a, int32_t b) {
    float x = AMotionEvent_getAxisValue(e, a, 0);
    float y = AMotionEvent_getAxisValue(e, b, 0);
    return x > y ? x : y;
}

static int32_t on_input(struct android_app* app, AInputEvent* e) {
    (void)app;
    const int32_t source = AInputEvent_getSource(e);
    if (AInputEvent_getType(e) == AINPUT_EVENT_TYPE_KEY) {
        const int32_t code = AKeyEvent_getKeyCode(e);
        const int32_t action = AKeyEvent_getAction(e);
        if (code == AKEYCODE_VOLUME_UP || code == AKEYCODE_VOLUME_DOWN || code == AKEYCODE_VOLUME_MUTE) {
            return 0; // let the system change the volume
        }
        if (action == AKEY_EVENT_ACTION_DOWN || action == AKEY_EVENT_ACTION_UP) {
            push(CC_EVENT_KEY, code, action == AKEY_EVENT_ACTION_DOWN ? 1.0f : 0.0f, 0);
        }
        return 1;
    }
    if (AInputEvent_getType(e) != AINPUT_EVENT_TYPE_MOTION) return 0;

    if ((source & AINPUT_SOURCE_JOYSTICK) == AINPUT_SOURCE_JOYSTICK ||
        (source & AINPUT_SOURCE_GAMEPAD) == AINPUT_SOURCE_GAMEPAD) {
        push(CC_EVENT_AXIS, 0, AMotionEvent_getAxisValue(e, AMOTION_EVENT_AXIS_X, 0), 0);
        push(CC_EVENT_AXIS, 1, AMotionEvent_getAxisValue(e, AMOTION_EVENT_AXIS_Y, 0), 0);
        push(CC_EVENT_AXIS, 2, AMotionEvent_getAxisValue(e, AMOTION_EVENT_AXIS_Z, 0), 0);
        push(CC_EVENT_AXIS, 3, AMotionEvent_getAxisValue(e, AMOTION_EVENT_AXIS_RZ, 0), 0);
        // Controllers report triggers as LTRIGGER/RTRIGGER or BRAKE/GAS.
        push(CC_EVENT_AXIS, 4, max_axis(e, AMOTION_EVENT_AXIS_LTRIGGER, AMOTION_EVENT_AXIS_BRAKE), 0);
        push(CC_EVENT_AXIS, 5, max_axis(e, AMOTION_EVENT_AXIS_RTRIGGER, AMOTION_EVENT_AXIS_GAS), 0);
        push(CC_EVENT_HAT, 0, AMotionEvent_getAxisValue(e, AMOTION_EVENT_AXIS_HAT_X, 0),
             AMotionEvent_getAxisValue(e, AMOTION_EVENT_AXIS_HAT_Y, 0));
        return 1;
    }
    if ((source & AINPUT_SOURCE_TOUCHSCREEN) == AINPUT_SOURCE_TOUCHSCREEN ||
        (source & AINPUT_SOURCE_MOUSE) == AINPUT_SOURCE_MOUSE) {
        // The first finger acts as the mouse's left button.
        const int32_t action = AMotionEvent_getAction(e) & AMOTION_EVENT_ACTION_MASK;
        const float   x = AMotionEvent_getX(e, 0), y = AMotionEvent_getY(e, 0);
        push(CC_EVENT_TOUCH, action == AMOTION_EVENT_ACTION_UP || action == AMOTION_EVENT_ACTION_CANCEL ? 0 : 1, x, y);
        return 1;
    }
    return 0;
}

// ---- lifecycle ---------------------------------------------------------------

void android_main(struct android_app* app) {
    cc_app = app;
    app->onAppCmd = on_app_cmd;
    app->onInputEvent = on_input;
    start_logger();
    extract_shaders();
    ccMain();
}

int cc_poll(int timeout_ms) {
    int                         events;
    struct android_poll_source* source;
    int                         polled = 0;
    while (ALooper_pollOnce(polled ? 0 : timeout_ms, NULL, &events, (void**)&source) >= 0) {
        if (source) source->process(cc_app, source);
        polled = 1;
        if (cc_app->destroyRequested) break;
    }
    return cc_app->destroyRequested;
}

float cc_density(void) {
    return (float)AConfiguration_getDensity(cc_app->config) / 160.0f;
}

void cc_finish(void) {
    ANativeActivity_finish(cc_app->activity);
    while (!cc_poll(100)) {
    }
}
