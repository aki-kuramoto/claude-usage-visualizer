// The C side of the macOS window. Go calls these; gui_darwin.m implements
// them with AppKit.
#ifndef CUV_GUI_DARWIN_H
#define CUV_GUI_DARWIN_H

// cuvRun starts the application on the main thread and never returns.
void cuvRun(void);

// cuvUpdate hands the latest state, as JSON, to the window. It may be called
// from any thread; the string is copied before it returns.
void cuvUpdate(const char *stateJSON);

// cuvPreferredLanguages returns the OS's preferred languages, most preferred
// first, separated by newlines. The caller frees the result.
char *cuvPreferredLanguages(void);

#endif
