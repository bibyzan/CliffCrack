//go:build ios

// UIKit side of the iOS platform layer (Objective-C). UIKit owns the main
// thread; the game loop runs on a thread of its own and talks to this file
// through a locked event queue and a few getters. The view is backed by a
// CAMetalLayer, which the renderer makes its Vulkan (MoltenVK) surface from.
#include "ios.h"

#import <QuartzCore/CAMetalLayer.h>
#import <UIKit/UIKit.h>

#include <pthread.h>
#include <stdatomic.h>
#include <string.h>
#include <time.h>
#include <fcntl.h>
#include <unistd.h>

#include "_cgo_export.h"

static pthread_mutex_t cc_lock = PTHREAD_MUTEX_INITIALIZER;
static pthread_cond_t  cc_wake = PTHREAD_COND_INITIALIZER;
static CCEvent         cc_events[CC_MAX_EVENTS];
static int             cc_event_count;

static CAMetalLayer* cc_metal_layer;
static atomic_int    cc_is_active = 1;
static atomic_int    cc_width, cc_height;
static float         cc_pixels_per_point = 1;

static void push(int type, int code, float value, float value2, uint64_t id) {
    pthread_mutex_lock(&cc_lock);
    if (cc_event_count < CC_MAX_EVENTS) {
        cc_events[cc_event_count++] = (CCEvent){type, code, value, value2, id};
    }
    pthread_cond_signal(&cc_wake);
    pthread_mutex_unlock(&cc_lock);
}

int cc_take_events(CCEvent* out, int capacity) {
    pthread_mutex_lock(&cc_lock);
    int n = cc_event_count < capacity ? cc_event_count : capacity;
    memcpy(out, cc_events, sizeof(CCEvent) * (size_t)n);
    memmove(cc_events, cc_events + n, sizeof(CCEvent) * (size_t)(cc_event_count - n));
    cc_event_count -= n;
    pthread_mutex_unlock(&cc_lock);
    return n;
}

void cc_wait(int timeout_ms) {
    struct timespec until;
    clock_gettime(CLOCK_REALTIME, &until);
    until.tv_sec += timeout_ms / 1000;
    until.tv_nsec += (long)(timeout_ms % 1000) * 1000000L;
    if (until.tv_nsec >= 1000000000L) {
        until.tv_sec++;
        until.tv_nsec -= 1000000000L;
    }
    pthread_mutex_lock(&cc_lock);
    if (cc_event_count == 0) pthread_cond_timedwait(&cc_wake, &cc_lock, &until);
    pthread_mutex_unlock(&cc_lock);
}

void* cc_layer(void) { return (__bridge void*)cc_metal_layer; }
int   cc_active(void) { return atomic_load(&cc_is_active); }
float cc_scale(void) { return cc_pixels_per_point; }

void cc_size(int* width, int* height) {
    *width = atomic_load(&cc_width);
    *height = atomic_load(&cc_height);
}

// ---- view --------------------------------------------------------------------

@interface CCView : UIView
@end

@implementation CCView

+ (Class)layerClass {
    return [CAMetalLayer class];
}

- (void)layoutSubviews {
    [super layoutSubviews];
    CGSize size = self.bounds.size;
    CGFloat scale = self.window.windowScene.screen.nativeScale ?: 1;
    self.contentScaleFactor = scale;
    CAMetalLayer* layer = (CAMetalLayer*)self.layer;
    layer.contentsScale = scale;
    layer.drawableSize = CGSizeMake(size.width * scale, size.height * scale);
    cc_pixels_per_point = (float)scale;
    atomic_store(&cc_width, (int)(size.width * scale));
    atomic_store(&cc_height, (int)(size.height * scale));
    push(CC_EVENT_RESIZE, 0, 0, 0, 0);
}

// Every finger is reported with its own id (the UITouch lives as long as
// the finger is down), in drawable pixels. The phases match input.TouchPhase.
- (void)report:(NSSet<UITouch*>*)touches phase:(int)phase {
    const CGFloat scale = self.contentScaleFactor;
    for (UITouch* touch in touches) {
        CGPoint p = [touch locationInView:self];
        push(CC_EVENT_TOUCH, phase, (float)(p.x * scale), (float)(p.y * scale), (uint64_t)(uintptr_t)touch);
    }
}

- (void)touchesBegan:(NSSet<UITouch*>*)touches withEvent:(UIEvent*)event {
    [self report:touches phase:0];
}
- (void)touchesMoved:(NSSet<UITouch*>*)touches withEvent:(UIEvent*)event {
    [self report:touches phase:1];
}
- (void)touchesEnded:(NSSet<UITouch*>*)touches withEvent:(UIEvent*)event {
    [self report:touches phase:2];
}
- (void)touchesCancelled:(NSSet<UITouch*>*)touches withEvent:(UIEvent*)event {
    [self report:touches phase:2];
}

@end

// Fullscreen and sideways: no status bar, the home indicator fades out, and
// swipes from the edges go to the game first (a second swipe goes home).
@interface CCViewController : UIViewController
@end

@implementation CCViewController

- (void)loadView {
    CCView* view = [[CCView alloc] initWithFrame:CGRectZero]; // the window sizes it
    view.multipleTouchEnabled = YES;
    view.backgroundColor = UIColor.blackColor;
    cc_metal_layer = (CAMetalLayer*)view.layer;
    self.view = view;
}

- (BOOL)prefersStatusBarHidden {
    return YES;
}
- (BOOL)prefersHomeIndicatorAutoHidden {
    return YES;
}
- (UIRectEdge)preferredScreenEdgesDeferringSystemGestures {
    return UIRectEdgeAll;
}
- (UIInterfaceOrientationMask)supportedInterfaceOrientations {
    return UIInterfaceOrientationMaskLandscape;
}

@end

// ---- app ---------------------------------------------------------------------

static void* game_thread(void* arg) {
    (void)arg;
    ccMain();
    return NULL;
}

// UIKit's scene lifecycle (required from iOS 26): the app delegate only
// names the scene delegate (Info.plist does too), which makes the window.
@interface CCAppDelegate : UIResponder <UIApplicationDelegate>
@end

@implementation CCAppDelegate

- (UISceneConfiguration*)application:(UIApplication*)app
    configurationForConnectingSceneSession:(UISceneSession*)session
                                   options:(UISceneConnectionOptions*)options {
    UISceneConfiguration* config = [UISceneConfiguration configurationWithName:@"Default" sessionRole:session.role];
    config.delegateClass = NSClassFromString(@"CCSceneDelegate");
    return config;
}

@end

@interface CCSceneDelegate : UIResponder <UIWindowSceneDelegate>
@property(strong, nonatomic) UIWindow* window;
@end

@implementation CCSceneDelegate

- (void)scene:(UIScene*)scene
    willConnectToSession:(UISceneSession*)session
                 options:(UISceneConnectionOptions*)options {
    static int started;
    if (started) return; // one game, one window
    started = 1;

    self.window = [[UIWindow alloc] initWithWindowScene:(UIWindowScene*)scene];
    self.window.rootViewController = [CCViewController new];
    [self.window makeKeyAndVisible];
    [self.window layoutIfNeeded]; // the layer has its size before the game starts
    UIApplication.sharedApplication.idleTimerDisabled = YES; // no dimming mid-run

    // The game loop gets its own thread (with room for Go's cgo calls), so
    // UIKit's main thread stays free to deliver touches.
    pthread_attr_t attr;
    pthread_attr_init(&attr);
    pthread_attr_setstacksize(&attr, 8 << 20);
    pthread_t thread;
    pthread_create(&thread, &attr, game_thread, NULL);
    pthread_attr_destroy(&attr);
}

// iOS doesn't let a background app use the GPU: the game stops drawing as
// soon as it is no longer frontmost.
- (void)sceneWillResignActive:(UIScene*)scene {
    atomic_store(&cc_is_active, 0);
    push(CC_EVENT_FOCUS, 0, 0, 0, 0);
}

- (void)sceneDidBecomeActive:(UIScene*)scene {
    atomic_store(&cc_is_active, 1);
    push(CC_EVENT_FOCUS, 1, 0, 0, 0);
}

@end

// ---- log ---------------------------------------------------------------------

// On a phone, stdout and stderr go to Documents/cliffcrack.log (the last
// run's), since nothing reliably shows them live. Read it with:
//   xcrun devicectl device copy from --device <id> --domain-type appDataContainer
//     --domain-identifier <bundle id> --source Documents/cliffcrack.log --destination .
// Writes go straight to the file, so nothing is lost when the game exits
// right after an error. The Simulator keeps its console.
static void start_log(void) {
#if !TARGET_OS_SIMULATOR
    NSString* docs = NSSearchPathForDirectoriesInDomains(NSDocumentDirectory, NSUserDomainMask, YES).firstObject;
    if (!docs) return;
    NSString* path = [docs stringByAppendingPathComponent:@"cliffcrack.log"];
    int fd = open(path.fileSystemRepresentation, O_WRONLY | O_CREAT | O_TRUNC, 0644);
    if (fd < 0) return;
    setvbuf(stdout, NULL, _IOLBF, 0);
    setvbuf(stderr, NULL, _IONBF, 0);
    dup2(fd, STDOUT_FILENO);
    dup2(fd, STDERR_FILENO);
    close(fd);
#endif
}

int cc_run(int argc, char** argv) {
    start_log();
    @autoreleasepool {
        return UIApplicationMain(argc, argv, nil, NSStringFromClass([CCAppDelegate class]));
    }
}
