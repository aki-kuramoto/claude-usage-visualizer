// The macOS window, written with AppKit.
//
// Go owns all the logic. This file only draws the state that Go hands over
// as JSON (see app.State) and passes the user's actions back through the
// functions Go exports (goFetchNow, goSaveSettings, ...). Every piece of text
// comes from the state's "strings" table, in the language Go chose. Everything
// here runs on the main thread.

#import <Cocoa/Cocoa.h>
#import <UserNotifications/UserNotifications.h>
#include "gui_darwin.h"
#include "_cgo_export.h"

static const CGFloat kWidth = 380; // the whole window, frame included
static const CGFloat kMargin = 12;  // inside the contents' border

// textWidth is the width of the contents' text, which depends on the theme's
// frame; render sets it before building the contents.
static CGFloat textWidth = kWidth - 2 * kMargin;

// hidden is set by the environment variable CUV_HIDDEN, for checking the
// drawing during development (with CUV_SNAPSHOT_DIR) without disturbing the
// person at the machine: the application then never shows a window, never
// comes to the front and has no Dock icon.
static BOOL hidden;

// --- colours from the theme -------------------------------------------------

// hexColor reads "#RRGGBB" from the theme, or gives nil (do not draw).
static NSColor *hexColor(id value) {
    if (![value isKindOfClass:[NSString class]] || [value length] != 7 || ![value hasPrefix:@"#"]) {
        return nil;
    }
    unsigned rgb = 0;
    if (![[NSScanner scannerWithString:[value substringFromIndex:1]] scanHexInt:&rgb]) {
        return nil;
    }
    return [NSColor colorWithSRGBRed:((rgb >> 16) & 0xFF) / 255.0
                               green:((rgb >> 8) & 0xFF) / 255.0
                                blue:(rgb & 0xFF) / 255.0
                               alpha:1];
}

static CGFloat themeNumber(NSDictionary *theme, NSString *key) {
    id v = theme[key];
    return [v isKindOfClass:[NSNumber class]] ? [v doubleValue] : 0;
}

// --- a horizontal bar -------------------------------------------------------

// The bar is drawn inside a taller view, so that the time marker can stick
// out above and below it, with room above for the usage-days triangle and
// below for the days-ahead triangle.
static const CGFloat kBarHeight = 10;
static const CGFloat kMarkerOverhang = 4;
static const CGFloat kTriangleHeight = 8;
static const CGFloat kTriangleWidth = 10;
// kBarBottom is where the bar starts, leaving room for the triangle below.
static const CGFloat kBarBottom = kTriangleHeight + 2;

@interface CUVBarView : NSView
@property(nonatomic) double fraction;
@property(nonatomic, strong) NSColor *color;
// The theme's colours: the empty part, its outline (nil for none), the slits,
// the dots and the markers.
@property(nonatomic, strong) NSColor *trackColor;
@property(nonatomic, strong) NSColor *outlineColor;
@property(nonatomic, strong) NSColor *slitColor;
@property(nonatomic, strong) NSColor *dotColor;
@property(nonatomic, strong) NSColor *markerColor;
// marker is how much of the limit's period has passed, from 0 to 1, drawn as
// a vertical line across the bar; negative means no marker.
@property(nonatomic) double marker;
// daysMarker is the same counted by the weights of days.setay, drawn as a
// small triangle above the bar pointing down at it; negative means none.
@property(nonatomic) double daysMarker;
// aheadMarker is the same judged by the days still ahead, drawn as a small
// triangle below the bar pointing up at it; negative means none.
@property(nonatomic) double aheadMarker;
// slits are where the local calendar turns over (midnight, or the hour for
// the five-hour limit), cut into the bar in the contents' background colour;
// dots are the period's equal parts from the reset time, as small dots just
// below the bar. Both are shares of the period from 0 to 1.
@property(nonatomic, strong) NSArray<NSNumber *> *slits;
@property(nonatomic, strong) NSArray<NSNumber *> *dots;
@end

@implementation CUVBarView
- (instancetype)init {
    if ((self = [super initWithFrame:NSZeroRect])) {
        _marker = -1;
        _daysMarker = -1;
        _aheadMarker = -1;
    }
    return self;
}
- (NSSize)intrinsicContentSize {
    return NSMakeSize(NSViewNoIntrinsicMetric, kBarBottom + kBarHeight + kTriangleHeight + 2);
}
// x is where a fraction of the period falls along the bar.
- (CGFloat)xFor:(double)f {
    double m = f > 1 ? 1 : f;
    return MIN(MAX(self.bounds.size.width * m, 1), self.bounds.size.width - 1);
}
- (void)drawRect:(NSRect)dirty {
    // The view is not flipped: y grows upwards, so the bar sits near the
    // bottom and the triangle above it. The bar is square, like the
    // progress bars of the classic Mac OS.
    NSRect r = NSMakeRect(0, kBarBottom, self.bounds.size.width, kBarHeight);
    [(self.trackColor ?: [NSColor quaternaryLabelColor]) setFill];
    NSRectFill(r);
    double f = self.fraction < 0 ? 0 : (self.fraction > 1 ? 1 : self.fraction);
    if (f > 0) {
        NSRect filled = r;
        filled.size.width = MAX(2, round(r.size.width * f));
        [self.color setFill];
        NSRectFill(filled);
    }
    // The ticks go under the outline and the markers, so that both stay
    // readable.
    [(self.slitColor ?: [NSColor windowBackgroundColor]) setFill];
    for (NSNumber *f in self.slits) {
        CGFloat x = r.size.width * f.doubleValue;
        NSRectFill(NSMakeRect(x - 0.75, r.origin.y, 1.5, r.size.height));
    }
    if (self.outlineColor) {
        [self.outlineColor setFill];
        NSFrameRect(r);
    }
    [(self.dotColor ?: [NSColor tertiaryLabelColor]) setFill];
    for (NSNumber *f in self.dots) {
        CGFloat x = r.size.width * f.doubleValue;
        [[NSBezierPath bezierPathWithOvalInRect:NSMakeRect(x - 1.2, r.origin.y - 4.4, 2.4, 2.4)] fill];
    }

    [(self.markerColor ?: [NSColor labelColor]) setFill];
    if (self.marker >= 0) {
        CGFloat x = [self xFor:self.marker];
        NSRectFill(NSMakeRect(x - 1, kBarBottom - kMarkerOverhang, 2, kBarHeight + 2 * kMarkerOverhang));
    }
    if (self.daysMarker >= 0) {
        CGFloat x = MIN(MAX([self xFor:self.daysMarker], kTriangleWidth / 2), self.bounds.size.width - kTriangleWidth / 2);
        CGFloat tip = kBarBottom + kBarHeight + 1;
        NSBezierPath *t = [NSBezierPath bezierPath];
        [t moveToPoint:NSMakePoint(x, tip)];
        [t lineToPoint:NSMakePoint(x - kTriangleWidth / 2, tip + kTriangleHeight)];
        [t lineToPoint:NSMakePoint(x + kTriangleWidth / 2, tip + kTriangleHeight)];
        [t closePath];
        [t fill];
    }
    if (self.aheadMarker >= 0) {
        CGFloat x = MIN(MAX([self xFor:self.aheadMarker], kTriangleWidth / 2), self.bounds.size.width - kTriangleWidth / 2);
        CGFloat tip = kBarBottom - 1;
        NSBezierPath *t = [NSBezierPath bezierPath];
        [t moveToPoint:NSMakePoint(x, tip)];
        [t lineToPoint:NSMakePoint(x - kTriangleWidth / 2, tip - kTriangleHeight)];
        [t lineToPoint:NSMakePoint(x + kTriangleWidth / 2, tip - kTriangleHeight)];
        [t closePath];
        [t fill];
    }
}
@end

// --- small helpers ----------------------------------------------------------

static NSTextField *makeLabel(NSString *text, CGFloat size, NSColor *color, BOOL bold) {
    NSTextField *l = [NSTextField labelWithString:text ?: @""];
    l.font = bold ? [NSFont boldSystemFontOfSize:size] : [NSFont systemFontOfSize:size];
    l.textColor = color;
    l.lineBreakMode = NSLineBreakByWordWrapping;
    l.usesSingleLineMode = NO;
    l.preferredMaxLayoutWidth = textWidth;
    return l;
}

static NSDate *parseTime(id value) {
    if (![value isKindOfClass:[NSString class]] || [value length] == 0) {
        return nil;
    }
    static NSISO8601DateFormatter *f;
    static NSISO8601DateFormatter *fractional;
    if (!f) {
        f = [[NSISO8601DateFormatter alloc] init];
        fractional = [[NSISO8601DateFormatter alloc] init];
        fractional.formatOptions = NSISO8601DateFormatWithInternetDateTime | NSISO8601DateFormatWithFractionalSeconds;
    }
    return [f dateFromString:value] ?: [fractional dateFromString:value];
}

// formatTime shows a time in the machine's local time zone, in the given
// locale, with the hour written the way the locale usually writes it:
// "10/17(金) 1:30" in Japanese, "Fri, 10/17, 1:30 AM" in English.
static NSString *formatTime(NSDate *d, NSString *locale) {
    if (!d) {
        return @"-";
    }
    static NSDateFormatter *f;
    static NSString *fLocale;
    if (!f || ![fLocale isEqualToString:locale]) {
        f = [[NSDateFormatter alloc] init];
        f.locale = [NSLocale localeWithLocaleIdentifier:locale];
        [f setLocalizedDateFormatFromTemplate:@"MdEjm"];
        fLocale = [locale copy];
    }
    return [f stringFromDate:d];
}

// fill puts value in place of {name} in text.
static NSString *fill(NSString *text, NSString *name, NSString *value) {
    return [text stringByReplacingOccurrencesOfString:[NSString stringWithFormat:@"{%@}", name]
                                           withString:value ?: @""];
}

// --- the window's own frame, title bar and menu bar ----------------------------

// The usage window has no title bar from macOS: it draws its own, in the look
// the theme describes (see the Go package theme), so that the window looks
// the same on every OS and keeps its presence when it is behind other windows
// in the compact view. CUVFrameView, the window's content view, draws the
// frame, the title bar and the menu bar's background; the boxes and the menu
// bar's items are views of their own, so that they can be pressed and be
// found by accessibility tools; the contents sit in a CUVContentView.

@class CUVFrameView;

// CUVWindow is borderless, which macOS does not let become the key window or
// close and minimize from the menus unless told.
@interface CUVWindow : NSWindow
@end

@implementation CUVWindow
- (BOOL)canBecomeKeyWindow {
    return YES;
}
- (BOOL)canBecomeMainWindow {
    return YES;
}
// The Window menu's Close (Cmd+W) and the close box both come here, and ask
// the delegate first, as a window with a title bar from macOS would.
- (void)performClose:(id)sender {
    id<NSWindowDelegate> d = self.delegate;
    if ([d respondsToSelector:@selector(windowShouldClose:)] && ![d windowShouldClose:self]) {
        return;
    }
    [self close];
}
- (void)performMiniaturize:(id)sender {
    [self miniaturize:sender];
}
- (BOOL)validateMenuItem:(NSMenuItem *)item {
    if (item.action == @selector(performClose:) || item.action == @selector(performMiniaturize:)) {
        return YES;
    }
    return [super validateMenuItem:item];
}
@end

@interface CUVFrameView : NSView
@property(nonatomic, strong) NSDictionary *theme;
@property(nonatomic, copy) NSString *title;
@property(nonatomic) BOOL compact;
@property(nonatomic, copy) void (^onDoubleClick)(void);
// forcedLook, when set to "active" or "inactive", draws that look whether
// or not the window is the key window (for snapshots).
@property(nonatomic, copy) NSString *forcedLook;
// The boxes, which the stripes leave gaps around.
@property(nonatomic, strong) NSView *closeBox;
@property(nonatomic, strong) NSView *shadeBox;
- (NSDictionary *)look;
@end

// lookFor finds the theme's look for the window a view is in: the active one
// for the key window, the inactive one otherwise.
static NSDictionary *lookFor(NSView *view) {
    NSView *v = view;
    while (v && ![v isKindOfClass:[CUVFrameView class]]) {
        v = v.superview;
    }
    return [(CUVFrameView *)v look];
}

static NSDictionary *themeFor(NSView *view) {
    NSView *v = view;
    while (v && ![v isKindOfClass:[CUVFrameView class]]) {
        v = v.superview;
    }
    return [(CUVFrameView *)v theme];
}

// fillLine fills a one-point line, if the colour is to be drawn.
static void fillLine(NSColor *c, NSRect r) {
    if (c) {
        [c setFill];
        NSRectFill(r);
    }
}

// CUVBoxView is the close box (an X inside) or the window-shade box (two
// lines inside). Like a button of the classic Mac OS, it acts when the mouse
// is released over it, and looks pressed while the mouse is held down on it.
typedef NS_ENUM(NSInteger, CUVBoxKind) { CUVBoxClose, CUVBoxShade };

@interface CUVBoxView : NSView
@property(nonatomic) CUVBoxKind kind;
@property(nonatomic) BOOL pressed;
@property(nonatomic, copy) NSString *label;
@property(nonatomic, copy) void (^action)(void);
@end

@implementation CUVBoxView
- (BOOL)isFlipped {
    return YES;
}
- (BOOL)mouseDownCanMoveWindow {
    return NO;
}
// The boxes work on a window behind others with the first click, as the
// buttons of macOS's own title bars do.
- (BOOL)acceptsFirstMouse:(NSEvent *)event {
    return YES;
}
- (void)drawRect:(NSRect)dirty {
    NSDictionary *look = lookFor(self);
    NSDictionary *theme = themeFor(self);
    NSRect r = self.bounds;
    CGFloat w = r.size.width, h = r.size.height;
    fillLine(hexColor(self.pressed ? look[@"box_pressed_fill"] : look[@"box_fill"]), r);
    if (!self.pressed) {
        NSColor *light = hexColor(look[@"box_bevel_light"]), *dark = hexColor(look[@"box_bevel_dark"]);
        fillLine(light, NSMakeRect(1, 1, w - 2, 1));
        fillLine(light, NSMakeRect(1, 1, 1, h - 2));
        fillLine(dark, NSMakeRect(1, h - 2, w - 2, 1));
        fillLine(dark, NSMakeRect(w - 2, 1, 1, h - 2));
    }
    NSColor *stroke = hexColor(look[@"box_stroke"]);
    if (stroke) {
        [stroke setFill];
        NSFrameRect(r);
    }

    CGFloat in = themeNumber(theme, @"glyph_inset");
    NSColor *glyph = hexColor(look[@"glyph"]);
    if (self.pressed && [look[@"box_pressed_fill"] isEqual:look[@"glyph"]]) {
        glyph = hexColor(look[@"box_fill"]); // keep the mark visible on a dark press
    }
    [glyph ?: [NSColor blackColor] set];
    if (self.kind == CUVBoxClose) {
        NSBezierPath *x = [NSBezierPath bezierPath];
        x.lineWidth = 1.2;
        [x moveToPoint:NSMakePoint(in, in)];
        [x lineToPoint:NSMakePoint(w - in, h - in)];
        [x moveToPoint:NSMakePoint(w - in, in)];
        [x lineToPoint:NSMakePoint(in, h - in)];
        [x stroke];
    } else {
        CGFloat mid = floor(h / 2);
        NSRectFill(NSMakeRect(in, mid - 1, w - 2 * in, 1));
        NSRectFill(NSMakeRect(in, mid + 1, w - 2 * in, 1));
    }
}
- (void)mouseDown:(NSEvent *)event {
    self.pressed = YES;
    self.needsDisplay = YES;
    BOOL inside = YES;
    while (YES) {
        NSEvent *e = [self.window nextEventMatchingMask:NSEventMaskLeftMouseDragged | NSEventMaskLeftMouseUp];
        inside = NSPointInRect([self convertPoint:e.locationInWindow fromView:nil], self.bounds);
        if (e.type == NSEventTypeLeftMouseUp) {
            break;
        }
        if (inside != self.pressed) {
            self.pressed = inside;
            self.needsDisplay = YES;
        }
    }
    self.pressed = NO;
    self.needsDisplay = YES;
    if (inside && self.action) {
        self.action();
    }
}
- (BOOL)isAccessibilityElement {
    return YES;
}
- (NSAccessibilityRole)accessibilityRole {
    return NSAccessibilityButtonRole;
}
- (NSString *)accessibilityLabel {
    return self.label;
}
- (BOOL)accessibilityPerformPress {
    if (self.action) {
        self.action();
    }
    return YES;
}
@end

// CUVMenuTitleView is one item of the menu bar inside the window. Pressing
// it opens its menu below it.
@interface CUVMenuTitleView : NSView
@property(nonatomic, copy) NSString *title;
@property(nonatomic, strong) NSMenu *dropMenu;
@property(nonatomic) BOOL open;
- (NSFont *)font;
@end

@implementation CUVMenuTitleView
- (BOOL)isFlipped {
    return YES;
}
- (BOOL)mouseDownCanMoveWindow {
    return NO;
}
- (BOOL)acceptsFirstMouse:(NSEvent *)event {
    return YES;
}
- (NSFont *)font {
    CGFloat size = themeNumber(themeFor(self), @"menu_font_size");
    return [NSFont systemFontOfSize:size > 0 ? size : 12];
}
- (void)drawRect:(NSRect)dirty {
    NSDictionary *theme = themeFor(self);
    NSColor *text = hexColor(lookFor(self)[@"menu_text"]);
    if (self.open) {
        fillLine(hexColor(theme[@"menu_highlight"]), self.bounds);
        text = hexColor(theme[@"menu_highlight_text"]);
    }
    NSDictionary *attrs = @{NSFontAttributeName : self.font, NSForegroundColorAttributeName : text ?: [NSColor blackColor]};
    NSSize size = [self.title sizeWithAttributes:attrs];
    [self.title drawAtPoint:NSMakePoint(themeNumber(theme, @"menu_padding"), floor((self.bounds.size.height - size.height) / 2))
             withAttributes:attrs];
}
- (void)popUp {
    self.open = YES;
    [self display];
    [self.dropMenu popUpMenuPositioningItem:nil atLocation:NSMakePoint(0, self.bounds.size.height) inView:self];
    self.open = NO;
    self.needsDisplay = YES;
}
- (void)mouseDown:(NSEvent *)event {
    [self popUp];
}
- (BOOL)isAccessibilityElement {
    return YES;
}
- (NSAccessibilityRole)accessibilityRole {
    return NSAccessibilityMenuButtonRole;
}
- (NSString *)accessibilityLabel {
    return self.title;
}
- (BOOL)accessibilityPerformPress {
    [self popUp];
    return YES;
}
@end

// CUVContentView holds the contents, on the theme's content colour inside a
// one-point border.
@interface CUVContentView : NSView
@end

@implementation CUVContentView
- (BOOL)isFlipped {
    return YES;
}
- (BOOL)mouseDownCanMoveWindow {
    return NO;
}
- (void)drawRect:(NSRect)dirty {
    fillLine(hexColor(themeFor(self)[@"content"]), self.bounds);
    NSColor *border = hexColor(lookFor(self)[@"content_border"]);
    if (border) {
        [border setFill];
        NSFrameRect(self.bounds);
    }
}
@end

@implementation CUVFrameView
- (BOOL)isFlipped {
    return YES;
}
- (BOOL)mouseDownCanMoveWindow {
    return NO; // only the title bar moves the window, handled below
}
- (BOOL)acceptsFirstMouse:(NSEvent *)event {
    return YES;
}
- (NSDictionary *)look {
    NSString *which = self.forcedLook ?: (self.window.isKeyWindow ? @"active" : @"inactive");
    NSDictionary *look = self.theme[which];
    return [look isKindOfClass:[NSDictionary class]] ? look : @{};
}
- (NSFont *)titleFont {
    CGFloat size = themeNumber(self.theme, @"title_font_size");
    size = size > 0 ? size : 12;
    return [self.theme[@"title_bold"] boolValue] ? [NSFont boldSystemFontOfSize:size] : [NSFont systemFontOfSize:size];
}
// titleRect is where the title goes: centred on the window when it fits,
// otherwise in the room between the boxes, where it is cut off.
- (NSRect)titleRect {
    NSDictionary *t = self.theme;
    CGFloat w = self.bounds.size.width;
    CGFloat gap = themeNumber(t, @"box_gap") + themeNumber(t, @"title_gap");
    CGFloat left = NSMaxX(self.closeBox.frame) + gap, right = NSMinX(self.shadeBox.frame) - gap;
    CGFloat textW = ceil([self.title sizeWithAttributes:@{NSFontAttributeName : self.titleFont}].width);
    CGFloat tw = MIN(textW, MAX(right - left, 0));
    CGFloat x = MIN(MAX(round(w / 2 - tw / 2), left), right - tw);
    CGFloat h = ceil(self.titleFont.ascender - self.titleFont.descender);
    CGFloat mid = themeNumber(t, @"box_top") + themeNumber(t, @"box_size") / 2;
    return NSMakeRect(x, round(mid - h / 2), tw, h);
}
- (void)drawRect:(NSRect)dirty {
    NSDictionary *t = self.theme, *look = self.look;
    CGFloat w = self.bounds.size.width, h = self.bounds.size.height;
    CGFloat titleBar = themeNumber(t, @"title_bar_height");
    fillLine(hexColor(look[@"frame"]), self.bounds);

    // The stripes, with gaps around the boxes and the title.
    NSRect title = self.title.length > 0 ? self.titleRect : NSZeroRect;
    CGFloat boxGap = themeNumber(t, @"box_gap"), titleGap = themeNumber(t, @"title_gap");
    NSMutableArray<NSValue *> *gaps = [NSMutableArray array];
    for (NSView *box in @[ self.closeBox, self.shadeBox ]) {
        [gaps addObject:[NSValue valueWithRange:NSMakeRange(NSMinX(box.frame) - boxGap, box.frame.size.width + 2 * boxGap)]];
    }
    if (title.size.width > 0) {
        [gaps addObject:[NSValue valueWithRange:NSMakeRange(NSMinX(title) - titleGap, title.size.width + 2 * titleGap)]];
    }
    [gaps sortUsingComparator:^NSComparisonResult(NSValue *a, NSValue *b) {
        return a.rangeValue.location < b.rangeValue.location ? NSOrderedAscending : NSOrderedDescending;
    }];
    NSArray *stripes = [look[@"stripes"] isKindOfClass:[NSArray class]] ? look[@"stripes"] : @[];
    CGFloat side = themeNumber(t, @"stripe_side");
    CGFloat top = themeNumber(t, @"stripe_top"), bottom = themeNumber(t, @"stripe_bottom");
    for (CGFloat y = top; stripes.count > 0 && y < bottom; y++) {
        NSColor *c = hexColor(stripes[(NSUInteger)(y - top) % stripes.count]);
        if (!c) {
            continue;
        }
        [c setFill];
        CGFloat x = side;
        for (NSValue *g in gaps) {
            NSRange r = g.rangeValue;
            if ((CGFloat)r.location > x) {
                NSRectFill(NSMakeRect(x, y, MIN((CGFloat)r.location, w - side) - x, 1));
            }
            x = MAX(x, (CGFloat)NSMaxRange(r));
        }
        if (x < w - side) {
            NSRectFill(NSMakeRect(x, y, w - side - x, 1));
        }
    }

    if (title.size.width > 0) {
        NSMutableParagraphStyle *p = [[NSMutableParagraphStyle alloc] init];
        p.lineBreakMode = NSLineBreakByTruncatingTail;
        [self.title drawWithRect:title
                         options:NSStringDrawingUsesLineFragmentOrigin | NSStringDrawingTruncatesLastVisibleLine
                      attributes:@{
                          NSFontAttributeName : self.titleFont,
                          NSForegroundColorAttributeName : hexColor(look[@"title"]) ?: [NSColor blackColor],
                          NSParagraphStyleAttributeName : p,
                      }
                         context:nil];
    }

    fillLine(hexColor(look[@"title_separator"]), NSMakeRect(0, titleBar - 1, w, 1));
    NSColor *light = hexColor(look[@"bevel_light"]), *dark = hexColor(look[@"bevel_dark"]);
    fillLine(light, NSMakeRect(1, 1, w - 2, 1));
    fillLine(light, NSMakeRect(1, 1, 1, h - 2));
    fillLine(dark, NSMakeRect(1, h - 2, w - 2, 1));
    fillLine(dark, NSMakeRect(w - 2, 1, 1, h - 2));
    NSColor *border = hexColor(look[@"border"]);
    if (border) {
        [border setFill];
        NSFrameRect(self.bounds);
    }
}
// Dragging the title bar moves the window; double-clicking it switches to
// and from the compact view, like the window shade of the classic Mac OS.
- (void)mouseDown:(NSEvent *)event {
    NSPoint p = [self convertPoint:event.locationInWindow fromView:nil];
    if (p.y >= themeNumber(self.theme, @"title_bar_height")) {
        return;
    }
    if (event.clickCount == 2) {
        if (self.onDoubleClick) {
            self.onDoubleClick();
        }
        return;
    }
    [self.window performWindowDragWithEvent:event];
}
@end

// --- the application --------------------------------------------------------

@interface CUVAppDelegate : NSObject <NSApplicationDelegate, NSWindowDelegate, UNUserNotificationCenterDelegate>
@property(nonatomic, strong) NSWindow *window;
@property(nonatomic, strong) NSStackView *stack;
@property(nonatomic, strong) NSDictionary *state;
@property(nonatomic, copy) NSString *lang; // the language the menus were built in
@property(nonatomic, strong) NSWindow *settingsWindow;
@property(nonatomic, strong) NSTextField *intervalField;
@property(nonatomic, strong) NSStepper *intervalStepper;
@property(nonatomic, strong) NSTextField *pathField;
@property(nonatomic, strong) NSButton *onTopCheck;
@property(nonatomic, strong) NSButton *expiryCheck;
@property(nonatomic, strong) NSPopUpButton *languagePopup;
@property(nonatomic, strong) NSPopUpButton *closePopup;
@property(nonatomic, strong) NSTextField *settingsError;
@property(nonatomic, strong) NSTextField *settingsInfo;
// notifiedLoggedOut is set once the "not logged in" notification has been
// shown, so that it is shown once for each time the login is lost.
@property(nonatomic) BOOL notifiedLoggedOut;
// compact is whether the compact view (one line of text) is shown. It starts
// as the start_compact setting says, once the first state arrives.
@property(nonatomic) BOOL compact;
@property(nonatomic) BOOL compactSettled;
@property(nonatomic, strong) NSButton *startCompactCheck;
@property(nonatomic, strong) NSPopUpButton *themePopup;
// The window's own frame (see CUVFrameView), the contents inside it, and
// the constraints that place the contents where the theme says.
@property(nonatomic, strong) CUVFrameView *frameView;
@property(nonatomic, strong) CUVContentView *container;
@property(nonatomic, strong) NSLayoutConstraint *containerTop;
@property(nonatomic, strong) NSLayoutConstraint *containerLeading;
@property(nonatomic, strong) NSLayoutConstraint *containerWidth;
@property(nonatomic, strong) NSArray<CUVMenuTitleView *> *menuTitles;
// chromeKey is the theme and the language the frame was last set up for.
@property(nonatomic, copy) NSString *chromeKey;
@end

static CUVAppDelegate *delegate;

@implementation CUVAppDelegate

// T looks a string up in the table Go sent.
- (NSString *)T:(NSString *)key {
    NSDictionary *strings = self.state[@"strings"];
    NSString *s = [strings isKindOfClass:[NSDictionary class]] ? strings[key] : nil;
    return s ?: key;
}

- (NSString *)appName {
    // The bundle's name inside the .app, the executable's name when run bare.
    return [[NSBundle mainBundle] objectForInfoDictionaryKey:@"CFBundleName"]
               ?: [[NSProcessInfo processInfo] processName];
}

- (NSString *)dateLocale {
    return self.state[@"date_locale"] ?: @"en_US";
}

- (NSDictionary *)settings {
    NSDictionary *s = self.state[@"settings"];
    return [s isKindOfClass:[NSDictionary class]] ? s : @{};
}

- (void)applicationDidFinishLaunching:(NSNotification *)n {
    [self buildWindow];
    // The time markers and the time left move on even when nothing new has
    // been read, so draw again now and then.
    [NSTimer scheduledTimerWithTimeInterval:30
                                    repeats:YES
                                      block:^(NSTimer *t) {
                                          if (self.window.isVisible) {
                                              [self render];
                                          }
                                      }];
    NSNotificationCenter *ws = [[NSWorkspace sharedWorkspace] notificationCenter];
    [ws addObserver:self selector:@selector(willSleep:) name:NSWorkspaceWillSleepNotification object:nil];
    [ws addObserver:self selector:@selector(didWake:) name:NSWorkspaceDidWakeNotification object:nil];
    [self render];
    [self placeForTesting];
    if (!hidden) {
        [self showWindow:nil];
        [NSApp activateIgnoringOtherApps:YES];
    }
}

// placeForTesting puts the window's top-left corner where the environment
// variable CUV_WINDOW_AT says ("x,y", in the coordinates System Events uses:
// from the top-left of the main screen, y growing downwards), before the
// window is first shown. During development it keeps a test window off the
// screen the person at the machine is working on.
- (void)placeForTesting {
    const char *at = getenv("CUV_WINDOW_AT");
    double x, y;
    if (!at || sscanf(at, "%lf,%lf", &x, &y) != 2) {
        return;
    }
    CGFloat top = NSMaxY(NSScreen.screens.firstObject.frame);
    [self.window setFrameTopLeftPoint:NSMakePoint(x, top - y)];
}

// Closing the window leaves the application running and recording, as macOS
// applications usually do; the Dock icon or the menu brings the window back.
- (BOOL)applicationShouldTerminateAfterLastWindowClosed:(NSApplication *)app {
    return NO;
}

- (BOOL)applicationShouldHandleReopen:(NSApplication *)app hasVisibleWindows:(BOOL)visible {
    if (!self.window.isVisible) {
        [self showWindow:nil];
    }
    return YES;
}

- (void)applicationWillTerminate:(NSNotification *)n {
    goWillTerminate();
}

- (void)willSleep:(NSNotification *)n {
    goNotePower("sleep");
}

- (void)didWake:(NSNotification *)n {
    goNotePower("wake");
}

// windowShouldClose decides, by the close_action setting, whether closing the
// usage window only closes it, quits the application, or asks which.
- (BOOL)windowShouldClose:(NSWindow *)sender {
    if (sender != self.window) {
        return YES;
    }
    NSString *action = self.settings[@"close_action"] ?: @"ask";
    if ([action isEqualToString:@"close"]) {
        return YES;
    }
    if ([action isEqualToString:@"quit"]) {
        dispatch_async(dispatch_get_main_queue(), ^{
            [NSApp terminate:nil];
        });
        return NO;
    }
    if (self.window.attachedSheet) {
        return NO; // already asking
    }
    NSAlert *alert = [[NSAlert alloc] init];
    alert.messageText = [self T:@"close_alert_title"];
    alert.informativeText = [NSString stringWithFormat:@"%@\n\n%@", [self T:@"close_alert_message"], [self T:@"close_alert_setting"]];
    [alert addButtonWithTitle:[self T:@"close_alert_quit"]];
    [alert addButtonWithTitle:[self T:@"close_alert_close"]];
    NSButton *cancel = [alert addButtonWithTitle:[self T:@"close_alert_cancel"]];
    cancel.keyEquivalent = @"\033";
    [alert beginSheetModalForWindow:self.window
                  completionHandler:^(NSModalResponse r) {
                      if (r == NSAlertFirstButtonReturn) {
                          [NSApp terminate:nil];
                      } else if (r == NSAlertSecondButtonReturn) {
                          [self.window close]; // close does not ask windowShouldClose again
                      }
                  }];
    return NO;
}

- (void)windowWillClose:(NSNotification *)n {
    if (n.object == self.window) {
        goNoteWindow("close");
    }
}

- (void)showWindow:(id)sender {
    BOOL wasVisible = self.window.isVisible;
    [self.window makeKeyAndOrderFront:nil];
    [NSApp activateIgnoringOtherApps:YES];
    if (!wasVisible) {
        goNoteWindow("show");
    }
}

- (void)fetchNow:(id)sender {
    goFetchNow();
}

// --- menu -------------------------------------------------------------------

- (void)buildMenu {
    NSMenu *bar = [[NSMenu alloc] init];
    NSString *name = [self appName];

    NSMenuItem *appItem = [[NSMenuItem alloc] init];
    NSMenu *appMenu = [[NSMenu alloc] initWithTitle:name];
    [appMenu addItemWithTitle:fill([self T:@"menu_about"], @"app", name)
                       action:@selector(orderFrontStandardAboutPanel:) keyEquivalent:@""];
    [appMenu addItem:[NSMenuItem separatorItem]];
    NSMenuItem *settings = [appMenu addItemWithTitle:[self T:@"menu_settings"] action:@selector(showSettings:) keyEquivalent:@","];
    settings.target = self;
    [appMenu addItem:[NSMenuItem separatorItem]];
    [appMenu addItemWithTitle:fill([self T:@"menu_hide"], @"app", name) action:@selector(hide:) keyEquivalent:@"h"];
    [appMenu addItemWithTitle:fill([self T:@"menu_quit"], @"app", name) action:@selector(terminate:) keyEquivalent:@"q"];
    appItem.submenu = appMenu;
    [bar addItem:appItem];

    // The edit menu makes copy and paste work in the settings window.
    NSMenuItem *editItem = [[NSMenuItem alloc] init];
    NSMenu *edit = [[NSMenu alloc] initWithTitle:[self T:@"menu_edit"]];
    [edit addItemWithTitle:[self T:@"menu_undo"] action:@selector(undo:) keyEquivalent:@"z"];
    [edit addItem:[NSMenuItem separatorItem]];
    [edit addItemWithTitle:[self T:@"menu_cut"] action:@selector(cut:) keyEquivalent:@"x"];
    [edit addItemWithTitle:[self T:@"menu_copy"] action:@selector(copy:) keyEquivalent:@"c"];
    [edit addItemWithTitle:[self T:@"menu_paste"] action:@selector(paste:) keyEquivalent:@"v"];
    [edit addItemWithTitle:[self T:@"menu_select_all"] action:@selector(selectAll:) keyEquivalent:@"a"];
    editItem.submenu = edit;
    [bar addItem:editItem];

    NSMenuItem *usageItem = [[NSMenuItem alloc] init];
    NSMenu *usage = [[NSMenu alloc] initWithTitle:[self T:@"menu_usage"]];
    NSMenuItem *fetch = [usage addItemWithTitle:[self T:@"menu_fetch_now"] action:@selector(fetchNow:) keyEquivalent:@"r"];
    fetch.target = self;
    NSMenuItem *show = [usage addItemWithTitle:[self T:@"menu_show"] action:@selector(showWindow:) keyEquivalent:@"1"];
    show.target = self;
    NSMenuItem *compact = [usage addItemWithTitle:[self T:@"menu_compact"] action:@selector(toggleCompact:) keyEquivalent:@"C"];
    compact.keyEquivalentModifierMask = NSEventModifierFlagCommand | NSEventModifierFlagShift;
    compact.target = self;
    usageItem.submenu = usage;
    [bar addItem:usageItem];

    NSMenuItem *windowItem = [[NSMenuItem alloc] init];
    NSMenu *windows = [[NSMenu alloc] initWithTitle:[self T:@"menu_window"]];
    [windows addItemWithTitle:[self T:@"menu_minimize"] action:@selector(performMiniaturize:) keyEquivalent:@"m"];
    [windows addItemWithTitle:[self T:@"menu_close"] action:@selector(performClose:) keyEquivalent:@"w"];
    windowItem.submenu = windows;
    [bar addItem:windowItem];
    NSApp.windowsMenu = windows;

    NSApp.mainMenu = bar;
}

// --- the usage window ---------------------------------------------------------

- (void)buildWindow {
    // Borderless: the title bar is drawn by CUVFrameView, not by macOS.
    CUVWindow *window = [[CUVWindow alloc] initWithContentRect:NSMakeRect(0, 0, kWidth, 200)
                                                     styleMask:NSWindowStyleMaskBorderless | NSWindowStyleMaskMiniaturizable
                                                       backing:NSBackingStoreBuffered
                                                         defer:NO];
    window.hasShadow = YES;
    window.opaque = YES;
    // The theme's colours are fixed and light, so the text on them must stay
    // dark even when macOS is in dark mode.
    window.appearance = [NSAppearance appearanceNamed:NSAppearanceNameAqua];
    self.window = window;
    self.window.releasedWhenClosed = NO;
    self.window.delegate = self;

    __weak CUVAppDelegate *weakSelf = self;
    CUVFrameView *frame = [[CUVFrameView alloc] initWithFrame:NSMakeRect(0, 0, kWidth, 200)];
    frame.onDoubleClick = ^{
        [weakSelf toggleCompact:nil];
    };
    window.contentView = frame;
    self.frameView = frame;

    CUVBoxView *close = [[CUVBoxView alloc] initWithFrame:NSZeroRect];
    close.kind = CUVBoxClose;
    close.action = ^{
        [weakSelf.window performClose:nil];
    };
    CUVBoxView *shade = [[CUVBoxView alloc] initWithFrame:NSZeroRect];
    shade.kind = CUVBoxShade;
    shade.action = ^{
        [weakSelf toggleCompact:nil];
    };
    [frame addSubview:close];
    [frame addSubview:shade];
    frame.closeBox = close;
    frame.shadeBox = shade;

    // The contents' height comes from the stack; the window is then sized
    // to it (see fitWindow), so nothing ties the contents to the window's
    // bottom edge.
    self.container = [[CUVContentView alloc] initWithFrame:NSZeroRect];
    self.container.translatesAutoresizingMaskIntoConstraints = NO;
    [frame addSubview:self.container];

    self.stack = [NSStackView stackViewWithViews:@[]];
    self.stack.orientation = NSUserInterfaceLayoutOrientationVertical;
    self.stack.alignment = NSLayoutAttributeLeading;
    self.stack.spacing = 6;
    self.stack.edgeInsets = NSEdgeInsetsMake(kMargin, kMargin, kMargin, kMargin);
    self.stack.translatesAutoresizingMaskIntoConstraints = NO;
    [self.container addSubview:self.stack];

    self.containerTop = [self.container.topAnchor constraintEqualToAnchor:frame.topAnchor];
    self.containerLeading = [self.container.leadingAnchor constraintEqualToAnchor:frame.leadingAnchor];
    self.containerWidth = [self.container.widthAnchor constraintEqualToConstant:kWidth];
    [NSLayoutConstraint activateConstraints:@[
        self.containerTop, self.containerLeading, self.containerWidth,
        [self.stack.leadingAnchor constraintEqualToAnchor:self.container.leadingAnchor],
        [self.stack.trailingAnchor constraintEqualToAnchor:self.container.trailingAnchor],
        [self.stack.topAnchor constraintEqualToAnchor:self.container.topAnchor],
        [self.stack.bottomAnchor constraintEqualToAnchor:self.container.bottomAnchor],
    ]];
    [self.window center];
    // A hidden run must not move where the real window opens next time.
    if (!hidden) {
        [self.window setFrameAutosaveName:@"usage"];
    }
}

- (NSDictionary *)theme {
    NSDictionary *t = self.state[@"theme"];
    return [t isKindOfClass:[NSDictionary class]] ? t : @{};
}

// applyTheme sets the frame up for the theme and the language: where the
// boxes, the menu bar and the contents go. It does nothing when neither has
// changed since last time.
- (void)applyTheme {
    NSDictionary *t = [self theme];
    NSString *key = [NSString stringWithFormat:@"%@/%@", t[@"id"] ?: @"", self.lang ?: @""];
    if ([key isEqualToString:self.chromeKey]) {
        return;
    }
    self.chromeKey = key;
    self.frameView.theme = t;

    CGFloat box = themeNumber(t, @"box_size"), boxTop = themeNumber(t, @"box_top"), boxSide = themeNumber(t, @"box_side");
    CUVBoxView *close = (CUVBoxView *)self.frameView.closeBox, *shade = (CUVBoxView *)self.frameView.shadeBox;
    close.frame = NSMakeRect(boxSide, boxTop, box, box);
    shade.frame = NSMakeRect(kWidth - boxSide - box, boxTop, box, box);
    close.label = [self T:@"menu_close"];
    shade.label = [self T:@"menu_compact"];

    CGFloat inset = themeNumber(t, @"frame_inset");
    self.containerTop.constant = themeNumber(t, @"title_bar_height") + themeNumber(t, @"menu_bar_height");
    self.containerLeading.constant = inset;
    self.containerWidth.constant = kWidth - 2 * inset;
    textWidth = kWidth - 2 * inset - 2 * kMargin;
    self.window.backgroundColor = hexColor(t[@"active"][@"frame"]) ?: [NSColor windowBackgroundColor];
    [self buildMenuStrip];
    [self refreshLook];
}

// buildMenuStrip makes the menu bar inside the window. It holds what every
// OS's window has; the same items stay in the macOS menu bar too, for those
// used to finding them there, for the compact view (which hides this menu
// bar) and for the keyboard shortcuts.
- (void)buildMenuStrip {
    for (NSView *v in self.menuTitles) {
        [v removeFromSuperview];
    }
    NSMenu *usage = [[NSMenu alloc] init];
    [self addItem:@"menu_fetch_now" action:@selector(fetchNow:) key:@"r" shift:NO to:usage];
    NSMenu *view = [[NSMenu alloc] init];
    [self addItem:@"menu_compact" action:@selector(toggleCompact:) key:@"C" shift:YES to:view];
    NSMenu *settings = [[NSMenu alloc] init];
    [self addItem:@"menu_settings" action:@selector(showSettings:) key:@"," shift:NO to:settings];

    NSDictionary *t = self.frameView.theme;
    CGFloat x = themeNumber(t, @"menu_left"), top = themeNumber(t, @"title_bar_height");
    CGFloat height = themeNumber(t, @"menu_bar_height"), pad = themeNumber(t, @"menu_padding");
    NSMutableArray<CUVMenuTitleView *> *titles = [NSMutableArray array];
    for (NSArray *m in @[ @[ @"menu_usage", usage ], @[ @"menu_view", view ], @[ @"menu_settings_menu", settings ] ]) {
        CUVMenuTitleView *v = [[CUVMenuTitleView alloc] initWithFrame:NSZeroRect];
        v.title = [self T:m[0]];
        v.dropMenu = m[1];
        [self.frameView addSubview:v]; // first, so that it finds the theme for its font
        CGFloat w = ceil([v.title sizeWithAttributes:@{NSFontAttributeName : v.font}].width) + 2 * pad;
        v.frame = NSMakeRect(x, top, w, height);
        v.hidden = self.compact;
        x += w;
        [titles addObject:v];
    }
    self.menuTitles = titles;
}

- (void)addItem:(NSString *)key action:(SEL)action key:(NSString *)k shift:(BOOL)shift to:(NSMenu *)menu {
    NSMenuItem *item = [menu addItemWithTitle:[self T:key] action:action keyEquivalent:k];
    if (shift) {
        item.keyEquivalentModifierMask = NSEventModifierFlagCommand | NSEventModifierFlagShift;
    }
    item.target = self;
}

// refreshLook redraws the frame, as when the window comes in front or goes
// behind.
- (void)refreshLook {
    self.frameView.needsDisplay = YES;
    for (NSView *v in self.frameView.subviews) {
        v.needsDisplay = YES;
    }
    if (getenv("CUV_SNAPSHOT_DIR")) {
        dispatch_async(dispatch_get_main_queue(), ^{
            [self snapshot];
        });
    }
}

// snapshot writes the window as the application draws it to PNGs in the
// directory CUV_SNAPSHOT_DIR names, for checking the drawing during
// development: the usage window in both looks (in front and behind), named
// <theme>-<full|compact>-<active|inactive>.png, and the settings window,
// named settings-<language>.png. It draws the windows itself, so it needs
// no permission to record the screen, and works with CUV_HIDDEN, when no
// window is on the screen at all.
- (void)snapshot {
    NSString *dir = [NSString stringWithUTF8String:getenv("CUV_SNAPSHOT_DIR")];
    for (NSString *look in @[ @"active", @"inactive" ]) {
        self.frameView.forcedLook = look;
        NSString *name = [NSString stringWithFormat:@"%@-%@-%@.png", self.theme[@"id"] ?: @"none",
                                                    self.compact ? @"compact" : @"full", look];
        [self writeView:self.frameView to:[dir stringByAppendingPathComponent:name]];
    }
    self.frameView.forcedLook = nil;

    [self loadSettings];
    NSView *settings = self.settingsWindow.contentView;
    [settings layoutSubtreeIfNeeded];
    [self.settingsWindow setContentSize:settings.fittingSize];
    [settings layoutSubtreeIfNeeded];
    [self writeView:settings to:[dir stringByAppendingPathComponent:[NSString stringWithFormat:@"settings-%@.png", self.lang]]];
}

- (void)writeView:(NSView *)v to:(NSString *)path {
    NSBitmapImageRep *rep = [v bitmapImageRepForCachingDisplayInRect:v.bounds];
    [v cacheDisplayInRect:v.bounds toBitmapImageRep:rep];
    [[rep representationUsingType:NSBitmapImageFileTypePNG properties:@{}] writeToFile:path atomically:YES];
}

- (void)windowDidBecomeKey:(NSNotification *)n {
    if (n.object == self.window) {
        [self refreshLook];
    }
}

- (void)windowDidResignKey:(NSNotification *)n {
    if (n.object == self.window) {
        [self refreshLook];
    }
}

- (void)applyJSON:(NSString *)json {
    NSData *data = [json dataUsingEncoding:NSUTF8StringEncoding];
    id obj = [NSJSONSerialization JSONObjectWithData:data options:0 error:nil];
    if (![obj isKindOfClass:[NSDictionary class]]) {
        return;
    }
    self.state = obj;
    [self render];
}

- (void)addFullWidth:(NSView *)v spacingAfter:(CGFloat)after {
    [self.stack addArrangedSubview:v];
    [v.widthAnchor constraintEqualToConstant:textWidth].active = YES;
    if (after >= 0) {
        [self.stack setCustomSpacing:after afterView:v];
    }
}

- (void)render {
    if (!self.stack) {
        return; // the window does not exist yet; render again once it does
    }

    // The language may have changed: rebuild what holds text outside the
    // usage window's contents.
    NSString *lang = self.state[@"lang"] ?: @"";
    if (![lang isEqualToString:self.lang ?: @"-"]) {
        self.lang = lang;
        [self buildMenu];
        if (self.settingsWindow) {
            [self.settingsWindow close];
            self.settingsWindow = nil; // rebuilt in the new language when next opened
        }
    }
    [self applyTheme];
    self.window.title = [self T:@"window_title"];
    self.frameView.title = self.window.title;
    self.window.level = [self.settings[@"always_on_top"] boolValue] ? NSFloatingWindowLevel : NSNormalWindowLevel;

    for (NSView *v in [self.stack.arrangedSubviews copy]) {
        [self.stack removeArrangedSubview:v];
        [v removeFromSuperview];
    }

    if (!self.compactSettled && self.state) {
        self.compactSettled = YES;
        self.compact = [self.settings[@"start_compact"] boolValue];
    }
    self.frameView.compact = self.compact;
    self.container.hidden = self.compact;
    for (NSView *v in self.menuTitles) {
        v.hidden = self.compact;
    }
    if (self.compact) {
        [self renderCompact];
        [self fillSettingsInfo];
        return;
    }

    NSDictionary *s = self.state;
    NSString *status = s[@"status"] ?: @"pending";
    BOOL fetching = [s[@"fetching"] boolValue];
    NSArray *limits = [s[@"limits"] isKindOfClass:[NSArray class]] ? s[@"limits"] : @[];
    BOOL failed = [status isEqualToString:@"failed"];
    NSString *fetchingNote = fetching ? [self T:@"fetching_suffix"] : @"";
    NSString *locale = [self dateLocale];

    // The heading says what the bars below mean, so that an old value is
    // never mistaken for the current one, and, after a failure, what went
    // wrong and what to do about it.
    NSString *kind = s[@"error_kind"] ?: @"other";
    if ([status isEqualToString:@"pending"]) {
        [self addFullWidth:makeLabel([[self T:@"pending"] stringByAppendingString:fetchingNote], 15,
                                     [NSColor secondaryLabelColor], YES)
              spacingAfter:-1];
    } else if (failed) {
        NSString *titleKey = [NSString stringWithFormat:@"err_%@_title", kind];
        NSString *bodyKey = [NSString stringWithFormat:@"err_%@_body", kind];
        [self addFullWidth:makeLabel([[self T:titleKey] stringByAppendingString:fetchingNote], 15,
                                     [NSColor systemRedColor], YES)
              spacingAfter:-1];
        NSString *body = [self T:bodyKey];
        if (body.length > 0 && ![body isEqualToString:bodyKey]) {
            [self addFullWidth:makeLabel(body, 12, [NSColor labelColor], NO) spacingAfter:-1];
        }
        NSButton *action = nil;
        if ([kind isEqualToString:@"not_logged_in"]) {
            action = [NSButton buttonWithTitle:[self T:@"login_button"] target:self action:@selector(openLogin:)];
        } else if ([kind isEqualToString:@"not_found"] || [kind isEqualToString:@"start"]) {
            action = [NSButton buttonWithTitle:[self T:@"open_settings_button"] target:self action:@selector(showSettings:)];
        }
        if (action) {
            [self.stack addArrangedSubview:action];
        }
        NSString *when = limits.count > 0
                             ? fill([self T:@"stale_note"], @"time", formatTime(parseTime(s[@"last_success_at"]), locale))
                             : [self T:@"failed_never"];
        [self addFullWidth:makeLabel(when, 12, [NSColor secondaryLabelColor], NO) spacingAfter:-1];
    } else {
        NSString *when = [fill([self T:@"read_at"], @"time", formatTime(parseTime(s[@"last_success_at"]), locale))
            stringByAppendingString:fetchingNote];
        [self addFullWidth:makeLabel(when, 12, [NSColor secondaryLabelColor], NO) spacingAfter:-1];
    }
    if (failed && [s[@"error"] length] > 0) {
        [self addFullWidth:makeLabel(s[@"error"], 11, [NSColor tertiaryLabelColor], NO) spacingAfter:-1];
    }
    // The login's expiry, when the check found something to say.
    NSDictionary *exp = [s[@"login_expiry"] isKindOfClass:[NSDictionary class]] ? s[@"login_expiry"] : nil;
    if ([exp[@"status"] isEqualToString:@"days"]) {
        NSString *msg = fill([self T:@"expiry_days"], @"days", [exp[@"days_left"] stringValue] ?: @"?");
        [self addFullWidth:makeLabel(msg, 12, [NSColor systemOrangeColor], YES) spacingAfter:-1];
        [self.stack addArrangedSubview:[NSButton buttonWithTitle:[self T:@"login_button"] target:self action:@selector(openLogin:)]];
    } else if ([exp[@"status"] isEqualToString:@"trust_needed"]) {
        [self addFullWidth:makeLabel([self T:@"expiry_trust"], 11, [NSColor secondaryLabelColor], NO) spacingAfter:-1];
        [self.stack addArrangedSubview:[NSButton buttonWithTitle:[self T:@"trust_button"] target:self action:@selector(openTrust:)]];
    }

    char *daysErr = goDaysError();
    if (daysErr) {
        NSString *msg = fill([self T:@"days_error"], @"error", [NSString stringWithUTF8String:daysErr]);
        free(daysErr);
        [self addFullWidth:makeLabel(msg, 11, [NSColor systemRedColor], NO) spacingAfter:-1];
    }

    [self updateAttention];
    if (limits.count > 0) {
        [self.stack setCustomSpacing:14 afterView:self.stack.arrangedSubviews.lastObject];
    }

    NSDictionary *theme = [self theme];
    for (NSDictionary *l in limits) {
        long pct = [l[@"used_pct"] longValue];
        NSColor *textColor = failed ? [NSColor secondaryLabelColor] : [NSColor labelColor];

        NSTextField *label = makeLabel(l[@"label"], 13, textColor, YES);
        NSTextField *percent = makeLabel([NSString stringWithFormat:@"%ld%%", pct], 13, textColor, YES);
        percent.alignment = NSTextAlignmentRight;
        // The name takes the room left over, which puts the percentage at
        // the right edge.
        [label setContentHuggingPriority:NSLayoutPriorityDefaultLow forOrientation:NSLayoutConstraintOrientationHorizontal];
        [percent setContentHuggingPriority:NSLayoutPriorityRequired forOrientation:NSLayoutConstraintOrientationHorizontal];
        NSStackView *top = [NSStackView stackViewWithViews:@[ label, percent ]];
        top.orientation = NSUserInterfaceLayoutOrientationHorizontal;
        top.distribution = NSStackViewDistributionFill;
        [self addFullWidth:top spacingAfter:4];

        NSDate *reset = parseTime(l[@"resets_at"]);
        double window = [l[@"window_seconds"] doubleValue];
        NSTimeInterval left = reset ? [reset timeIntervalSinceNow] : 0;
        double elapsed = -1;
        if (reset && window > 0) {
            elapsed = MIN(MAX(1 - left / window, 0), 1);
        }

        CUVBarView *bar = [[CUVBarView alloc] init];
        bar.fraction = pct / 100.0;
        NSString *fillKey = failed ? @"bar_stale" : (pct >= 90 ? @"bar_danger" : (pct >= 70 ? @"bar_warn" : @"bar_fill"));
        bar.color = hexColor(theme[fillKey]) ?: [NSColor systemGrayColor];
        bar.trackColor = hexColor(theme[@"bar_track"]);
        bar.outlineColor = hexColor(theme[@"bar_outline"]);
        bar.slitColor = hexColor(theme[@"content"]);
        bar.dotColor = hexColor(theme[@"tick_dot"]);
        bar.markerColor = hexColor(theme[@"marker"]);
        bar.marker = elapsed;
        bar.slits = [l[@"calendar_ticks"] isKindOfClass:[NSArray class]] ? l[@"calendar_ticks"] : @[];
        bar.dots = [l[@"equal_ticks"] isKindOfClass:[NSArray class]] ? l[@"equal_ticks"] : @[];
        double byDays = reset ? goDaysElapsed(reset.timeIntervalSince1970, window) : -1;
        bar.daysMarker = byDays;
        double ahead = reset ? goDaysAhead(reset.timeIntervalSince1970, window) : -1;
        bar.aheadMarker = ahead;
        [self addFullWidth:bar spacingAfter:2];

        NSMutableArray<NSString *> *parts = [NSMutableArray array];
        [parts addObject:reset ? fill([self T:@"resets_at"], @"time", formatTime(reset, locale)) : [self T:@"no_reset"]];
        if (reset && left > 0) {
            [parts addObject:[self remaining:left]];
        }
        if (elapsed >= 0) {
            [parts addObject:fill([self T:@"elapsed"], @"pct", [NSString stringWithFormat:@"%.0f", floor(elapsed * 100)])];
        }
        if (byDays >= 0) {
            [parts addObject:fill([self T:@"elapsed_days"], @"pct", [NSString stringWithFormat:@"%.0f", floor(byDays * 100)])];
        }
        if (ahead >= 0) {
            [parts addObject:fill([self T:@"elapsed_ahead"], @"pct", [NSString stringWithFormat:@"%.0f", floor(ahead * 100)])];
        }
        [self addFullWidth:makeLabel([parts componentsJoinedByString:@" · "], 11, [NSColor secondaryLabelColor], NO)
              spacingAfter:14];
    }

    [self fitWindow];
    [self fillSettingsInfo];
}

// fitWindow sizes the window to its contents (or to its title bar alone in
// the compact view), keeping the title bar where it was.
- (void)fitWindow {
    NSDictionary *t = self.frameView.theme;
    CGFloat height = themeNumber(t, @"title_bar_height");
    if (!self.compact) {
        [self.container layoutSubtreeIfNeeded];
        height += themeNumber(t, @"menu_bar_height") + self.container.fittingSize.height + themeNumber(t, @"frame_inset");
    }
    NSRect frame = self.window.frame;
    CGFloat top = NSMaxY(frame);
    frame.size = NSMakeSize(kWidth, height);
    frame.origin.y = top - height;
    [self.window setFrame:frame display:YES];
    [self refreshLook];
}

// renderCompact draws the compact view, like the window shade of the
// classic Mac OS: the window keeps its width but shrinks to its title bar,
// and the title says "5h 23%  ·  週 42%" (the five-hour limit and the weekly
// limit for all models; a title that does not fit is cut off). When
// nothing could be read, the title says only that, so that old values are
// never taken for current ones.
- (void)renderCompact {
    NSDictionary *s = self.state;
    NSString *status = s[@"status"] ?: @"pending";
    NSArray *limits = [s[@"limits"] isKindOfClass:[NSArray class]] ? s[@"limits"] : @[];

    NSString *title;
    if ([status isEqualToString:@"pending"]) {
        title = [self T:@"compact_pending"];
    } else if ([status isEqualToString:@"failed"]) {
        title = [self T:@"compact_failed"];
    } else {
        NSMutableArray<NSString *> *parts = [NSMutableArray array];
        for (NSDictionary *l in limits) {
            if ([l[@"primary"] boolValue]) {
                [parts addObject:[NSString stringWithFormat:@"%@ %ld%%", l[@"short"], [l[@"used_pct"] longValue]]];
            }
        }
        title = [parts componentsJoinedByString:@"  ·  "];
    }
    self.window.title = title;
    self.frameView.title = title;
    [self updateAttention];
    [self fitWindow];
}

- (void)toggleCompact:(id)sender {
    self.compact = !self.compact;
    self.compactSettled = YES;
    goNoteWindow(self.compact ? "compact" : "expand");
    [self render];
}

- (BOOL)validateMenuItem:(NSMenuItem *)item {
    if (item.action == @selector(toggleCompact:)) {
        item.state = self.compact ? NSControlStateValueOn : NSControlStateValueOff;
    }
    return YES;
}

// updateAttention says "not logged in" outside the window too, whichever
// view is shown: a badge on the Dock icon, and a notification once each time
// the login is lost. Not being logged in needs the user.
- (void)updateAttention {
    NSString *status = self.state[@"status"] ?: @"pending";
    BOOL loggedOut = [status isEqualToString:@"failed"] && [self.state[@"error_kind"] isEqualToString:@"not_logged_in"];
    NSApp.dockTile.badgeLabel = loggedOut ? @"!" : nil;
    if (loggedOut && !self.notifiedLoggedOut) {
        self.notifiedLoggedOut = YES;
        [self notify:[self T:@"notify_logged_out_title"] body:[self T:@"notify_logged_out_body"]];
    } else if (!loggedOut && ![status isEqualToString:@"pending"]) {
        self.notifiedLoggedOut = NO;
    }
}



- (void)openTrust:(id)sender {
    char *err = goOpenTrust();
    if (err) {
        NSAlert *alert = [[NSAlert alloc] init];
        alert.messageText = fill([self T:@"trust_failed"], @"error", [NSString stringWithUTF8String:err]);
        free(err);
        [alert beginSheetModalForWindow:self.window completionHandler:nil];
    }
}

- (void)openLogin:(id)sender {
    char *err = goOpenLogin();
    if (err) {
        NSAlert *alert = [[NSAlert alloc] init];
        alert.messageText = fill([self T:@"login_failed"], @"error", [NSString stringWithUTF8String:err]);
        free(err);
        [alert beginSheetModalForWindow:self.window completionHandler:nil];
    }
}

// notify shows a macOS notification. It needs the application to be a
// bundle (a bare executable has no identity to notify under), and the user's
// permission, which macOS asks for the first time.
- (void)notify:(NSString *)title body:(NSString *)body {
    if (![[NSBundle mainBundle] bundleIdentifier]) {
        return;
    }
    UNUserNotificationCenter *center = [UNUserNotificationCenter currentNotificationCenter];
    center.delegate = self;
    [center requestAuthorizationWithOptions:UNAuthorizationOptionAlert | UNAuthorizationOptionSound
                          completionHandler:^(BOOL granted, NSError *error) {
                              if (!granted) {
                                  return;
                              }
                              UNMutableNotificationContent *content = [[UNMutableNotificationContent alloc] init];
                              content.title = title;
                              content.body = body;
                              UNNotificationRequest *req = [UNNotificationRequest requestWithIdentifier:@"logged-out"
                                                                                                content:content
                                                                                                trigger:nil];
                              [center addNotificationRequest:req withCompletionHandler:nil];
                          }];
}

// Show the notification even while the application is in front.
- (void)userNotificationCenter:(UNUserNotificationCenter *)center
       willPresentNotification:(UNNotification *)notification
         withCompletionHandler:(void (^)(UNNotificationPresentationOptions))completionHandler {
    completionHandler(UNNotificationPresentationOptionBanner | UNNotificationPresentationOptionList);
}

// Clicking the notification brings the usage window forward.
- (void)userNotificationCenter:(UNUserNotificationCenter *)center
    didReceiveNotificationResponse:(UNNotificationResponse *)response
             withCompletionHandler:(void (^)(void))completionHandler {
    dispatch_async(dispatch_get_main_queue(), ^{
        [self showWindow:nil];
    });
    completionHandler();
}

// remaining says how long until a reset, roughly: days and hours, hours and
// minutes, or minutes.
- (NSString *)remaining:(NSTimeInterval)seconds {
    long s = (long)seconds;
    long d = s / 86400, h = s % 86400 / 3600, m = s % 3600 / 60;
    NSString *(^num)(long) = ^(long v) {
        return [NSString stringWithFormat:@"%ld", v];
    };
    if (d > 0) {
        return fill(fill([self T:@"remaining_dh"], @"d", num(d)), @"h", num(h));
    }
    if (h > 0) {
        return fill(fill([self T:@"remaining_hm"], @"h", num(h)), @"m", num(m));
    }
    return fill([self T:@"remaining_m"], @"m", num(m));
}

// --- the settings window ------------------------------------------------------

- (void)showSettings:(id)sender {
    [self loadSettings];
    [self.settingsWindow center];
    [self.settingsWindow makeKeyAndOrderFront:nil];
    [NSApp activateIgnoringOtherApps:YES];
}

// loadSettings builds the settings window if needed and fills it with the
// current settings, without showing it.
- (void)loadSettings {
    if (!self.settingsWindow) {
        [self buildSettings];
    }
    NSDictionary *settings = self.settings;
    long interval = [settings[@"interval_minutes"] longValue] ?: 5;
    self.intervalField.integerValue = interval;
    self.intervalStepper.integerValue = interval;
    self.pathField.stringValue = settings[@"claude_path"] ?: @"";
    self.onTopCheck.state = [settings[@"always_on_top"] boolValue] ? NSControlStateValueOn : NSControlStateValueOff;
    self.expiryCheck.state = [settings[@"login_expiry_check"] boolValue] ? NSControlStateValueOn : NSControlStateValueOff;
    self.startCompactCheck.state = [settings[@"start_compact"] boolValue] ? NSControlStateValueOn : NSControlStateValueOff;
    [self selectValue:settings[@"language"] in:self.languagePopup];
    [self selectValue:settings[@"close_action"] in:self.closePopup];
    [self selectValue:settings[@"theme"] in:self.themePopup];
    self.settingsError.stringValue = @"";
    [self fillSettingsInfo];
}

- (void)selectValue:(NSString *)value in:(NSPopUpButton *)popup {
    for (NSMenuItem *item in popup.itemArray) {
        if ([item.representedObject isEqual:value]) {
            [popup selectItem:item];
            return;
        }
    }
    [popup selectItemAtIndex:0];
}

- (NSPopUpButton *)popupWithChoices:(NSArray<NSArray<NSString *> *> *)choices {
    NSPopUpButton *p = [[NSPopUpButton alloc] initWithFrame:NSZeroRect pullsDown:NO];
    for (NSArray<NSString *> *c in choices) {
        [p addItemWithTitle:c[1]];
        p.lastItem.representedObject = c[0];
    }
    return p;
}

- (void)fillSettingsInfo {
    if (!self.settingsInfo) {
        return;
    }
    self.settingsInfo.stringValue = [NSString stringWithFormat:@"%@\n%@\n%@",
                                              fill([self T:@"settings_file"], @"path", self.state[@"settings_path"] ?: @"-"),
                                              fill([self T:@"days_file"], @"path", self.state[@"days_path"] ?: @"-"),
                                              fill([self T:@"logs_dir"], @"path", self.state[@"logs_dir"] ?: @"-")];
}

- (NSStackView *)row:(NSArray<NSView *> *)views {
    NSStackView *r = [NSStackView stackViewWithViews:views];
    r.orientation = NSUserInterfaceLayoutOrientationHorizontal;
    return r;
}

- (void)buildSettings {
    NSWindow *w = [[NSWindow alloc] initWithContentRect:NSMakeRect(0, 0, 480, 300)
                                              styleMask:NSWindowStyleMaskTitled | NSWindowStyleMaskClosable
                                                backing:NSBackingStoreBuffered
                                                  defer:NO];
    w.title = [self T:@"settings_title"];
    w.releasedWhenClosed = NO;
    self.settingsWindow = w;

    NSNumberFormatter *digits = [[NSNumberFormatter alloc] init];
    digits.numberStyle = NSNumberFormatterNoStyle;
    digits.allowsFloats = NO;

    self.intervalField = [NSTextField textFieldWithString:@""];
    self.intervalField.formatter = digits;
    [self.intervalField.widthAnchor constraintEqualToConstant:60].active = YES;
    self.intervalStepper = [[NSStepper alloc] init];
    self.intervalStepper.minValue = 1;
    self.intervalStepper.maxValue = 60;
    self.intervalStepper.increment = 1;
    self.intervalStepper.target = self;
    self.intervalStepper.action = @selector(stepperChanged:);
    NSStackView *intervalRow = [self row:@[
        [NSTextField labelWithString:[self T:@"interval_label"]], self.intervalField, self.intervalStepper,
        [NSTextField labelWithString:[self T:@"interval_unit"]]
    ]];

    self.pathField = [NSTextField textFieldWithString:@""];
    self.pathField.placeholderString = [self T:@"claude_path_hint"];
    NSStackView *pathRow = [self row:@[ [NSTextField labelWithString:[self T:@"claude_path_label"]], self.pathField ]];

    self.onTopCheck = [NSButton checkboxWithTitle:[self T:@"always_on_top"] target:nil action:nil];
    self.expiryCheck = [NSButton checkboxWithTitle:[self T:@"login_expiry_check"] target:nil action:nil];
    self.startCompactCheck = [NSButton checkboxWithTitle:[self T:@"start_compact"] target:nil action:nil];

    NSDictionary *names = self.state[@"language_names"];
    self.languagePopup = [self popupWithChoices:@[
        @[ @"system", [self T:@"language_system"] ],
        @[ @"ja", names[@"ja"] ?: @"ja" ],
        @[ @"en", names[@"en"] ?: @"en" ],
    ]];
    NSStackView *languageRow = [self row:@[ [NSTextField labelWithString:[self T:@"language_label"]], self.languagePopup ]];

    self.closePopup = [self popupWithChoices:@[
        @[ @"ask", [self T:@"close_ask"] ],
        @[ @"close", [self T:@"close_close"] ],
        @[ @"quit", [self T:@"close_quit"] ],
    ]];
    NSStackView *closeRow = [self row:@[ [NSTextField labelWithString:[self T:@"close_label"]], self.closePopup ]];

    NSMutableArray *themes = [NSMutableArray array];
    for (NSString *id in [self.state[@"theme_ids"] isKindOfClass:[NSArray class]] ? self.state[@"theme_ids"] : @[]) {
        [themes addObject:@[ id, [self T:[@"theme_" stringByAppendingString:id]] ]];
    }
    self.themePopup = [self popupWithChoices:themes];
    NSStackView *themeRow = [self row:@[ [NSTextField labelWithString:[self T:@"theme_label"]], self.themePopup ]];

    self.settingsError = makeLabel(@"", 12, [NSColor systemRedColor], NO);
    self.settingsError.preferredMaxLayoutWidth = 440;
    self.settingsInfo = makeLabel(@"", 11, [NSColor secondaryLabelColor], NO);
    self.settingsInfo.preferredMaxLayoutWidth = 440;
    self.settingsInfo.selectable = YES;

    NSButton *cancel = [NSButton buttonWithTitle:[self T:@"cancel"] target:self action:@selector(cancelSettings:)];
    cancel.keyEquivalent = @"\033";
    NSButton *save = [NSButton buttonWithTitle:[self T:@"save"] target:self action:@selector(saveSettings:)];
    save.keyEquivalent = @"\r";
    NSStackView *buttons = [self row:@[ cancel, save ]];

    NSStackView *form = [NSStackView stackViewWithViews:@[
        intervalRow, pathRow, self.onTopCheck, self.startCompactCheck, themeRow, languageRow, closeRow, self.expiryCheck,
        self.settingsError, self.settingsInfo, buttons
    ]];
    form.orientation = NSUserInterfaceLayoutOrientationVertical;
    form.alignment = NSLayoutAttributeLeading;
    form.spacing = 12;
    form.edgeInsets = NSEdgeInsetsMake(20, 20, 20, 20);
    form.translatesAutoresizingMaskIntoConstraints = NO;
    [w.contentView addSubview:form];
    [NSLayoutConstraint activateConstraints:@[
        [form.leadingAnchor constraintEqualToAnchor:w.contentView.leadingAnchor],
        [form.trailingAnchor constraintEqualToAnchor:w.contentView.trailingAnchor],
        [form.topAnchor constraintEqualToAnchor:w.contentView.topAnchor],
        [form.bottomAnchor constraintEqualToAnchor:w.contentView.bottomAnchor],
        [w.contentView.widthAnchor constraintEqualToConstant:480],
        [pathRow.widthAnchor constraintEqualToConstant:440],
        [buttons.trailingAnchor constraintEqualToAnchor:form.trailingAnchor constant:-20],
    ]];
}

- (void)stepperChanged:(id)sender {
    self.intervalField.integerValue = self.intervalStepper.integerValue;
}

- (void)cancelSettings:(id)sender {
    [self.settingsWindow close];
}

- (void)saveSettings:(id)sender {
    NSDictionary *values = @{
        @"interval_minutes" : @(self.intervalField.integerValue),
        @"claude_path" : self.pathField.stringValue ?: @"",
        // @YES and @NO, not @(a == b): the latter is a number, which JSON
        // writes as 1 or 0 rather than true or false.
        @"always_on_top" : self.onTopCheck.state == NSControlStateValueOn ? @YES : @NO,
        @"login_expiry_check" : self.expiryCheck.state == NSControlStateValueOn ? @YES : @NO,
        @"start_compact" : self.startCompactCheck.state == NSControlStateValueOn ? @YES : @NO,
        @"language" : self.languagePopup.selectedItem.representedObject ?: @"system",
        @"close_action" : self.closePopup.selectedItem.representedObject ?: @"ask",
        @"theme" : self.themePopup.selectedItem.representedObject ?: @"platinum",
    };
    NSData *data = [NSJSONSerialization dataWithJSONObject:values options:0 error:nil];
    NSString *json = [[NSString alloc] initWithData:data encoding:NSUTF8StringEncoding];
    char *err = goSaveSettings((char *)json.UTF8String);
    if (err) {
        self.settingsError.stringValue = fill([self T:@"save_failed"], @"error", [NSString stringWithUTF8String:err]);
        free(err);
        return;
    }
    [self.settingsWindow close];
}

@end

// --- entry points for Go ------------------------------------------------------

void cuvRun(void) {
    @autoreleasepool {
        [NSApplication sharedApplication];
        // A bare executable (not inside a .app) starts as a background
        // process; make it a normal application with a Dock icon and menus.
        hidden = getenv("CUV_HIDDEN") != NULL;
        [NSApp setActivationPolicy:hidden ? NSApplicationActivationPolicyAccessory : NSApplicationActivationPolicyRegular];
        delegate = [[CUVAppDelegate alloc] init];
        NSApp.delegate = delegate;
        [NSApp run];
    }
}

void cuvUpdate(const char *stateJSON) {
    NSString *json = [NSString stringWithUTF8String:stateJSON];
    dispatch_async(dispatch_get_main_queue(), ^{
        [delegate applyJSON:json];
    });
}

char *cuvPreferredLanguages(void) {
    @autoreleasepool {
        NSString *joined = [[NSLocale preferredLanguages] componentsJoinedByString:@"\n"];
        return strdup(joined.UTF8String);
    }
}
