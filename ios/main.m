// The app's entry point. The game is a static library (Go, with the UIKit
// glue in engine/platform/ios.m); UIKit takes over the main thread from here.
int cc_run(int argc, char** argv);

int main(int argc, char** argv) {
    return cc_run(argc, argv);
}
