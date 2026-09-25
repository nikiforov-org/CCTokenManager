#import <Cocoa/Cocoa.h>
#import <ServiceManagement/ServiceManagement.h>

extern char *cpSaveAll(char *profilesJSON, char *activeID);
extern char *cpTest(char *token);
extern char *cpApply(char *profilesJSON, char *activeID);
extern void  cpRevert(void);
extern void  cpSetLaunchAtLogin(void);

// Where the profiles' default folders live, as the Go side reports it.
static NSString *cp_profiles_dir = @"";
// The themes Claude Code offers, each a value and a name, and the one a new
// profile starts with, as the Go side lists them.
static NSArray  *cp_themes;
static NSString *cp_default_theme = @"";

@class CPSecretView;

static NSString      *cp_default_dir(NSString *name, NSString *pid);
static CGFloat        cp_baseline_in_box(NSTextView *tv);
static NSTextField   *cp_label(NSString *text);
static NSTextField   *cp_field(NSString *value);
static NSScrollView  *cp_textarea(CPSecretView **out);
static NSStackView   *cp_row(NSArray *views);
static CGFloat        cp_gap(void);

// The token's field. Out of focus it shows shading in place of the token, and
// token is what it stands for either way.
@interface CPSecretView : NSTextView
@property (copy, nonatomic) NSString *token;
@end

@implementation CPSecretView {
    NSString *_secret;
    BOOL _shown;
}

// A shade for each character of the token: an empty field stays empty.
static NSString *cp_noise(NSString *token) {
    __block NSUInteger n = 0;
    [token enumerateSubstringsInRange:NSMakeRange(0, token.length)
                              options:NSStringEnumerationByComposedCharacterSequences
                           usingBlock:^(NSString *c, NSRange r, NSRange e, BOOL *stop) { n++; }];
    return [@"" stringByPaddingToLength:n withString:@"░" startingAtIndex:0];
}

- (NSString *)token { return _shown ? [self string] : (_secret ?: @""); }

- (void)setToken:(NSString *)token {
    _secret = [token copy];
    [self setString:_shown ? _secret : cp_noise(_secret)];
}

// Swapping the token and the shading clears undo, so none reaches across.
- (void)show:(BOOL)shown {
    if (shown == _shown) return;
    if (!shown) _secret = [[self string] copy];
    _shown = shown;
    [self setString:shown ? _secret : cp_noise(_secret)];
    [[self undoManager] removeAllActionsWithTarget:self];
    [[self undoManager] removeAllActionsWithTarget:[self textStorage]];
}

- (BOOL)becomeFirstResponder {
    BOOL ok = [super becomeFirstResponder];
    if (ok) [self show:YES];
    return ok;
}

- (BOOL)resignFirstResponder {
    BOOL ok = [super resignFirstResponder];
    if (ok) [self show:NO];
    return ok;
}
@end

@interface CPController : NSObject <NSApplicationDelegate, NSWindowDelegate,
                                   NSTableViewDataSource, NSTableViewDelegate>
// The profile applied to Claude Code right now. It is not the selected
// one: selection browses, Apply profile decides.
@property (copy)   NSString    *appliedID;
// One detail view per profile, built from the same template. Nothing is shared
// between them, so switching is showing one and hiding the rest — there is no
// form to copy in and out, and nothing to get out of step.
@property (strong) NSView *detailHost;
@property (strong) NSMutableArray<NSMutableDictionary *> *rows;
@property (strong) NSTableView *table;
@property (strong) NSSegmentedControl *listButtons;
@property (strong) NSWindow    *window;
@property (strong) NSStatusItem *statusItem;
@property (assign) BOOL updatingTable;
@end

@implementation CPController

- (void)applicationWillTerminate:(NSNotification *)note { cpRevert(); }

- (BOOL)windowShouldClose:(NSWindow *)sender {
    [self.window orderOut:nil];
    [NSApp setActivationPolicy:NSApplicationActivationPolicyAccessory];
    return NO;
}

- (void)showWindow:(id)sender {
    [NSApp setActivationPolicy:NSApplicationActivationPolicyRegular];
    [self.window makeKeyAndOrderFront:nil];
    [NSApp activateIgnoringOtherApps:YES];
}

// Every outcome arrives as a dialog. A message over 70 characters is split:
// the first sentence is the headline, the rest the detail.
- (void)alertText:(NSString *)text ok:(BOOL)ok {
    NSString *head = text, *detail = @"";
    NSRange dot = [text rangeOfString:@". "];
    if (dot.location != NSNotFound && text.length > 70) {
        head = [text substringToIndex:dot.location + 1];
        detail = [text substringFromIndex:dot.location + 2];
    }
    NSAlert *a = [[NSAlert alloc] init];
    a.alertStyle = ok ? NSAlertStyleInformational : NSAlertStyleWarning;
    a.messageText = head;
    a.informativeText = detail;
    [a addButtonWithTitle:@"OK"];
    [self showWindow:nil];
    [a runModal];
}

// The bridge answers with {"ok":…,"text":…}. done hears how it went before the
// dialog comes up, so the window already shows what the dialog says.
- (void)report:(char *)r done:(void (^)(BOOL ok))done {
    NSDictionary *d = [NSJSONSerialization JSONObjectWithData:
        [NSData dataWithBytes:r length:strlen(r)] options:0 error:nil];
    free(r);
    BOOL ok = [d[@"ok"] boolValue];
    if (done) done(ok);
    [self alertText:d[@"text"] ok:ok];
}

// --- profile list ---

- (NSInteger)numberOfRowsInTableView:(NSTableView *)t { return self.rows.count; }

// The row carries a tick when it is the applied profile, the way a checked
// menu item does.
- (NSView *)tableView:(NSTableView *)t viewForTableColumn:(NSTableColumn *)col row:(NSInteger)row {
    NSTableCellView *cell = [t makeViewWithIdentifier:@"profile" owner:self];
    if (!cell) {
        cell = [[NSTableCellView alloc] init];
        cell.identifier = @"profile";

        // Read left to right: the mark comes before what it marks. Its place is
        // held whether or not it is shown, so the names stay in a column.
        NSImageView *tick = [NSImageView imageViewWithImage:
            [NSImage imageNamed:NSImageNameMenuOnStateTemplate]];
        [tick setTranslatesAutoresizingMaskIntoConstraints:NO];
        // The mark is the size of the mark. Slack in the row belongs to the
        // name, or a short one pushes the tick halfway across the list.
        [tick setContentHuggingPriority:NSLayoutPriorityRequired
                         forOrientation:NSLayoutConstraintOrientationHorizontal];
        [tick setContentCompressionResistancePriority:NSLayoutPriorityRequired
                                       forOrientation:NSLayoutConstraintOrientationHorizontal];

        NSTextField *tf = [NSTextField labelWithString:@""];
        // One line, cut with an ellipsis: a name too long for the list must not
        // widen it, wrap in it, or run off its edge.
        [tf setUsesSingleLineMode:YES];
        [tf setMaximumNumberOfLines:1];
        [tf setLineBreakMode:NSLineBreakByTruncatingTail];
        [[tf cell] setTruncatesLastVisibleLine:YES];
        [tf setContentCompressionResistancePriority:NSLayoutPriorityDefaultLow
                                     forOrientation:NSLayoutConstraintOrientationHorizontal];
        [tf setTranslatesAutoresizingMaskIntoConstraints:NO];

        [cell addSubview:tick];
        [cell addSubview:tf];
        // The gap between controls at both ends, or the mark sits on the edge
        // and the ellipsis is drawn past it; a window's edge margin is too wide.
        [NSLayoutConstraint activateConstraints:@[
            [tick.leadingAnchor constraintEqualToAnchor:cell.leadingAnchor
                                               constant:cp_gap()],
            [tf.leadingAnchor constraintEqualToAnchor:tick.trailingAnchor
                                             constant:cp_gap()],
            [cell.trailingAnchor constraintEqualToAnchor:tf.trailingAnchor
                                                constant:cp_gap()],
            [tick.centerYAnchor constraintEqualToAnchor:cell.centerYAnchor],
            [tf.centerYAnchor constraintEqualToAnchor:cell.centerYAnchor],
        ]];
        cell.textField = tf;
        cell.imageView = tick;
    }
    NSString *n = [self.rows[row][@"name"] stringValue];
    cell.textField.stringValue = n.length ? n : @"Untitled";

    NSString *rid = self.rows[row][@"id"];
    BOOL applied = rid.length && [rid isEqualToString:self.appliedID];
    [[cell imageView] setHidden:!applied];
    cell.toolTip = applied ? @"Applied to Claude Code" : nil;
    return cell;
}

- (void)tableViewSelectionDidChange:(NSNotification *)note {
    if (self.updatingTable) return;
    [self showSelected];
}

// showSelected reveals the selected row's detail view and redraws that row,
// so the list shows its name as it now stands.
- (void)showSelected {
    NSInteger row = [self.table selectedRow];
    for (NSUInteger i = 0; i < self.rows.count; i++) {
        BOOL shown = (NSInteger)i == row;
        [self.rows[i][@"view"] setHidden:!shown];
        // Return presses the button the profile on screen offers, and only it.
        [self.rows[i][@"apply"] setKeyEquivalent:shown ? @"\r" : @""];
    }
    if (row < 0) return;

    self.updatingTable = YES;
    [self.table reloadDataForRowIndexes:[NSIndexSet indexSetWithIndex:row]
                          columnIndexes:[NSIndexSet indexSetWithIndex:0]];
    self.updatingTable = NO;
}

// The buttons under the list: + adds a profile, − deletes the one on screen.
- (void)listAction:(NSSegmentedControl *)sender {
    if ([sender selectedSegment] == 0) {
        NSDictionary *blank = @{@"id": [[NSUUID UUID] UUIDString], @"name": @"New profile",
                                @"token": @"", @"dir": @"", @"theme": cp_default_theme};
        [self.rows addObject:[self buildDetailFor:blank]];
        [self reloadListSelecting:self.rows.count - 1];
        [self.window makeFirstResponder:self.rows.lastObject[@"name"]];
        return;
    }
    NSInteger row = [self.table selectedRow];
    NSAlert *a = [[NSAlert alloc] init];
    a.messageText = [NSString stringWithFormat:@"Delete profile “%@”?",
                     [self.rows[row][@"name"] stringValue]];
    a.informativeText = @"Its token is removed from this app.";
    [a addButtonWithTitle:@"Delete"];
    [a addButtonWithTitle:@"Cancel"];
    if ([a runModal] != NSAlertFirstButtonReturn) return;

    NSString *goneID = self.rows[row][@"id"];
    [[NSNotificationCenter defaultCenter] removeObserver:self.rows[row][@"watch"]];
    [self.rows[row][@"view"] removeFromSuperview];
    [self.rows removeObjectAtIndex:row];
    NSInteger sel = MIN(row, (NSInteger)self.rows.count - 1);
    // The tick cannot stay on a profile that is gone, nor move to one that is
    // not applied.
    if ([goneID isEqualToString:self.appliedID])
        self.appliedID = @"";
    [self reloadListSelecting:sel];
}

// fitColumn makes the table and its one column end where the list ends.
- (void)fitColumn {
    NSScrollView *sv = [self.table enclosingScrollView];
    CGFloat w = NSWidth([[sv contentView] bounds]);
    if (!sv || w <= 0 || [self.table numberOfRows] == 0) return;
    // A sidebar indents a row's contents without narrowing the column, so the
    // indent, AppKit's own, comes off both sides.
    CGFloat indent = NSMinX([self.table frameOfCellAtColumn:0 row:0]);
    [[self.table tableColumns][0] setWidth:MAX(w - indent * 2, 0)];
    [self.table setFrameSize:NSMakeSize(w, NSHeight([self.table frame]))];
}

- (void)reloadListSelecting:(NSInteger)row {
    self.updatingTable = YES;
    [self.table reloadData];
    [self fitColumn];
    [self.listButtons setEnabled:(self.rows.count > 1) forSegment:1];
    if (row >= 0 && row < (NSInteger)self.rows.count)
        [self.table selectRowIndexes:[NSIndexSet indexSetWithIndex:row]
                byExtendingSelection:NO];
    self.updatingTable = NO;

    // The window has to have room for everything in it. Growing is all that
    // happens here: a window the user has stretched keeps its size.
    NSView *cv = [self.window contentView];
    NSSize need = [cv fittingSize], have = [cv frame].size;
    [self.window setContentMinSize:need];
    if (need.width > have.width || need.height > have.height)
        [self.window setContentSize:NSMakeSize(MAX(need.width, have.width),
                                               MAX(need.height, have.height))];
    [self showSelected];
}

// buildDetailFor lays out one profile's fields. Every profile gets its own set
// from this template, so no value is ever shared between two of them.
- (NSMutableDictionary *)buildDetailFor:(NSDictionary *)p {
    NSTextField *name = cp_field(p[@"name"]);

    CPSecretView *token = nil;
    NSScrollView *tokenBox = cp_textarea(&token);
    [token setToken:p[@"token"]];
    NSTextField *tokenLabel = cp_label(@"OAuth token");

    // The profile's Claude Code config folder. Left empty it is the default one, named
    // after the profile, and the hint follows the name as it is typed.
    NSTextField *dir = cp_field(p[@"dir"]);
    NSString *pid = p[@"id"];
    [dir setPlaceholderString:cp_default_dir(p[@"name"], pid)];
    id watch = [[NSNotificationCenter defaultCenter]
        addObserverForName:NSControlTextDidChangeNotification object:name queue:nil
                usingBlock:^(NSNotification *note) {
                    // The list and the default folder follow the name as it is typed.
                    [dir setPlaceholderString:cp_default_dir([name stringValue], pid)];
                    [self showSelected];
                }];
    NSStackView *dirRow = cp_row(@[dir, [NSButton buttonWithTitle:@"Choose…" target:self
                                                            action:@selector(chooseDir:)]]);

    // The theme Claude Code shows while the profile is applied, from those it
    // offers.
    NSPopUpButton *theme = [[NSPopUpButton alloc] init];
    for (NSDictionary *t in cp_themes) {
        [theme addItemWithTitle:t[@"Name"]];
        [[theme lastItem] setRepresentedObject:t[@"Value"]];
    }
    [theme selectItemAtIndex:[theme indexOfItemWithRepresentedObject:p[@"theme"]]];

    // One grid for the whole form, so every label shares a column and lines up
    // with the control beside it.
    NSGridView *g = [NSGridView gridViewWithViews:@[
        @[cp_label(@"Profile name"),     name],
        @[tokenLabel,                    tokenBox],
        @[[NSGridCell emptyContentView], [NSButton buttonWithTitle:@"Test connection" target:self
                                                            action:@selector(test:)]],
        @[cp_label(@"Profile folder"),   dirRow],
        @[cp_label(@"Profile theme"),    theme],
    ]];
    [g setRowAlignment:NSGridRowAlignmentFirstBaseline];
    [[g columnAtIndex:0] setXPlacement:NSGridCellPlacementTrailing];
    [[g columnAtIndex:1] setXPlacement:NSGridCellPlacementFill];
    // A box of text has no single baseline, so its label is put on the first
    // line inside it: the top of the box is a border's width higher than that.
    [[g rowAtIndex:1] setRowAlignment:NSGridRowAlignmentNone];
    [[g rowAtIndex:1] setYPlacement:NSGridCellPlacementTop];
    NSGridCell *tokenCell = [g cellAtColumnIndex:0 rowIndex:1];
    [tokenCell setYPlacement:NSGridCellPlacementNone];
    [tokenCell setCustomPlacementConstraints:@[
        [[tokenLabel firstBaselineAnchor] constraintEqualToAnchor:[tokenBox topAnchor]
                                                         constant:cp_baseline_in_box(token)],
    ]];

    // Acting on the whole profile rather than on one field, these two stand
    // apart from the form's rows, in its bottom corner.
    NSButton *save = [NSButton buttonWithTitle:@"Save" target:self action:@selector(save:)];
    NSButton *apply = [NSButton buttonWithTitle:@"Apply profile" target:self
                                         action:@selector(applyProfile:)];
    NSStackView *actions = cp_row(@[]);
    [actions addView:save inGravity:NSStackViewGravityTrailing];
    [actions addView:apply inGravity:NSStackViewGravityTrailing];

    NSView *box = [[NSView alloc] init];
    [box setTranslatesAutoresizingMaskIntoConstraints:NO];
    [g setTranslatesAutoresizingMaskIntoConstraints:NO];
    [box addSubview:g];
    [box addSubview:actions];
    [self.detailHost addSubview:box];
    [NSLayoutConstraint activateConstraints:@[
        [box.topAnchor      constraintEqualToAnchor:self.detailHost.topAnchor],
        [box.leadingAnchor  constraintEqualToAnchor:self.detailHost.leadingAnchor],
        [box.trailingAnchor constraintEqualToAnchor:self.detailHost.trailingAnchor],
        [box.bottomAnchor   constraintEqualToAnchor:self.detailHost.bottomAnchor],

        [g.topAnchor      constraintEqualToAnchor:box.topAnchor],
        [g.leadingAnchor  constraintEqualToAnchor:box.leadingAnchor],
        [g.trailingAnchor constraintEqualToAnchor:box.trailingAnchor],

        [actions.leadingAnchor  constraintEqualToAnchor:box.leadingAnchor],
        [actions.trailingAnchor constraintEqualToAnchor:box.trailingAnchor],
        [actions.bottomAnchor   constraintEqualToAnchor:box.bottomAnchor],
        // the last block stands well clear of the buttons under it
        [actions.topAnchor      constraintGreaterThanOrEqualToAnchor:g.bottomAnchor
                                                            constant:cp_gap() * 4],
    ]];
    [box setHidden:YES];

    return [@{@"view": box, @"id": pid, @"name": name, @"token": token, @"dir": dir,
              @"theme": theme, @"watch": watch, @"apply": apply} mutableCopy];
}

// --- actions ---

// Values are read out of the controls, so what is on screen is what is stored.
- (NSString *)profilesJSON {
    NSMutableArray *out = [NSMutableArray array];
    for (NSMutableDictionary *r in self.rows)
        [out addObject:@{@"id":    r[@"id"],
                         @"name":  [r[@"name"] stringValue],
                         @"token": [(CPSecretView *)r[@"token"] token],
                         @"dir":   [r[@"dir"] stringValue],
                         @"theme": [[r[@"theme"] selectedItem] representedObject]}];
    NSData *d = [NSJSONSerialization dataWithJSONObject:out options:0 error:nil];
    return [[NSString alloc] initWithData:d encoding:NSUTF8StringEncoding];
}

// The file remembers the applied profile, to apply it at the next launch.
- (void)save:(id)sender {
    char *r = cpSaveAll((char *)[[self profilesJSON] UTF8String],
                        (char *)[self.appliedID UTF8String]);
    [self reloadListSelecting:[self.table selectedRow]];
    [self report:r done:nil];
}

// The disabled button is the only sign of work in progress; the answer comes
// back as a dialog.
- (void)runAsync:(NSButton *)b job:(char *(^)(void))job done:(void (^)(BOOL ok))done {
    b.enabled = NO;
    dispatch_async(dispatch_get_global_queue(QOS_CLASS_USER_INITIATED, 0), ^{
        char *r = job();
        dispatch_async(dispatch_get_main_queue(), ^{
            b.enabled = YES;
            [self report:r done:done];
        });
    });
}

- (void)test:(id)sender {
    NSInteger row = [self.table selectedRow];
    // Read the token from the field, not from the store: it may not be saved.
    NSString *tok = [(CPSecretView *)self.rows[row][@"token"] token];
    [self runAsync:sender job:^char *{ return cpTest((char *)[tok UTF8String]); } done:nil];
}

// The folder is picked the way any folder is: an open panel that can make a new
// one, starting where the profile's folder is, or would be.
- (void)chooseDir:(id)sender {
    NSTextField *dir = self.rows[[self.table selectedRow]][@"dir"];
    NSString *now = [[dir stringValue] length] ? [dir stringValue] : [dir placeholderString];

    NSOpenPanel *panel = [NSOpenPanel openPanel];
    [panel setCanChooseDirectories:YES];
    [panel setCanChooseFiles:NO];
    [panel setCanCreateDirectories:YES];
    // A folder that does not exist yet opens at its nearest parent that does.
    NSString *start = [now stringByExpandingTildeInPath];
    while (start.length > 1 && ![[NSFileManager defaultManager] fileExistsAtPath:start])
        start = [start stringByDeletingLastPathComponent];
    [panel setDirectoryURL:[NSURL fileURLWithPath:start isDirectory:YES]];
    if ([panel runModal] != NSModalResponseOK) return;
    [dir setStringValue:[[[panel URL] path] stringByAbbreviatingWithTildeInPath]];
}

// Applying switches Claude Code to the profile as it stands on screen. It does
// not save: Save is the button for that, and the tick shows which profile is
// applied right now, not which one the file remembers.
- (void)applyProfile:(id)sender {
    NSString *js = [self profilesJSON], *aid = self.rows[[self.table selectedRow]][@"id"];
    [self runAsync:sender job:^char *{
        return cpApply((char *)[js UTF8String], (char *)[aid UTF8String]);
    } done:^(BOOL ok) {
        // A profile that did not go through leaves none applied: ~/.claude is
        // back, and no row gets the tick.
        self.appliedID = ok ? aid : @"";
        [self reloadListSelecting:[self.table selectedRow]];
    }];
}
@end

// --- widgets ---

static NSFont *cp_mono(void) {
    return [NSFont monospacedSystemFontOfSize:[NSFont systemFontSize]
                                       weight:NSFontWeightRegular];
}

// The thickness of a bezelled scroll view's frame, asked of AppKit rather
// than assumed.
static CGFloat cp_scroll_border(void) {
    NSSize f = [NSScrollView frameSizeForContentSize:NSZeroSize
                            horizontalScrollerClass:nil
                              verticalScrollerClass:nil
                                         borderType:NSBezelBorder
                                        controlSize:NSControlSizeRegular
                                      scrollerStyle:[NSScroller preferredScrollerStyle]];
    return f.height / 2;
}

// Where the first line's baseline falls below the top of the box holding it.
static CGFloat cp_baseline_in_box(NSTextView *tv) {
    NSLayoutManager *lm = [[NSLayoutManager alloc] init];
    return cp_scroll_border() + [tv textContainerInset].height
         + [lm defaultBaselineOffsetForFont:[tv font]];
}

static NSTextField *cp_label(NSString *text) {
    NSTextField *l = [NSTextField labelWithString:text];
    [l setAlignment:NSTextAlignmentRight];
    [l setTextColor:[NSColor secondaryLabelColor]];
    // A label is the one thing that must never be shortened to fit: everything
    // beside it can give way instead.
    [l setContentCompressionResistancePriority:NSLayoutPriorityRequired
                                forOrientation:NSLayoutConstraintOrientationHorizontal];
    [l setContentHuggingPriority:NSLayoutPriorityRequired
                  forOrientation:NSLayoutConstraintOrientationHorizontal];
    return l;
}

static NSTextField *cp_field(NSString *value) {
    NSTextField *f = [NSTextField textFieldWithString:value ?: @""];
    [f setFont:cp_mono()];
    return f;
}

// Multi-line field, sized in lines and characters of the font it shows.
// Substitutions and autocorrect are off: they would corrupt a typed token.
static NSScrollView *cp_textarea(CPSecretView **out) {
    NSScrollView *sv = [[NSScrollView alloc] init];
    [sv setBorderType:NSBezelBorder]; [sv setHasVerticalScroller:YES];
    [sv setAutohidesScrollers:YES];

    CPSecretView *tv = [[CPSecretView alloc] init];
    [tv setMinSize:NSMakeSize(0, 0)];
    [tv setMaxSize:NSMakeSize(CGFLOAT_MAX, CGFLOAT_MAX)];
    [tv setVerticallyResizable:YES]; [tv setHorizontallyResizable:NO];
    [tv setAutoresizingMask:NSViewWidthSizable];
    [[tv textContainer] setWidthTracksTextView:YES];
    [tv setFont:cp_mono()];
    [tv setRichText:NO];
    [tv setAutomaticQuoteSubstitutionEnabled:NO];
    [tv setAutomaticDashSubstitutionEnabled:NO];
    [tv setAutomaticTextReplacementEnabled:NO];
    [tv setAutomaticSpellingCorrectionEnabled:NO];
    [tv setContinuousSpellCheckingEnabled:NO];
    [sv setDocumentView:tv];

    NSLayoutManager *lm = [[NSLayoutManager alloc] init];
    CGFloat line = [lm defaultLineHeightForFont:[tv font]];
    CGFloat ch = [cp_mono() maximumAdvancement].width;
    CGFloat border = cp_scroll_border() * 2 + [tv textContainerInset].height * 2;
    [NSLayoutConstraint activateConstraints:@[
        [[sv heightAnchor] constraintEqualToConstant:ceil(line * 4) + border],
        [[sv widthAnchor] constraintGreaterThanOrEqualToConstant:ceil(ch * 48) + border],
    ]];
    *out = tv;
    return sv;
}

// The folder a profile gets when none is chosen: its name made safe for a path,
// or its id when the name leaves nothing usable — the rule the Go side follows.
static NSString *cp_default_dir(NSString *name, NSString *pid) {
    NSString *n = [[name stringByReplacingOccurrencesOfString:@"/" withString:@"-"]
                   stringByTrimmingCharactersInSet:[NSCharacterSet whitespaceAndNewlineCharacterSet]];
    if (!n.length || [n isEqualToString:@"."] || [n isEqualToString:@".."])
        n = pid;
    return [[cp_profiles_dir stringByAppendingPathComponent:n] stringByAbbreviatingWithTildeInPath];
}

// The standard distance between two controls, as a stack view would use. It is
// the same answer every time, so it is asked for once.
static CGFloat cp_gap(void) {
    static CGFloat gap = 0;
    static dispatch_once_t once;
    dispatch_once(&once, ^{ gap = [[NSStackView stackViewWithViews:@[]] spacing]; });
    return gap;
}

// A line of controls at the standard spacing, sharing one baseline.
static NSStackView *cp_row(NSArray *views) {
    NSStackView *s = [NSStackView stackViewWithViews:views];
    [s setOrientation:NSUserInterfaceLayoutOrientationHorizontal];
    [s setAlignment:NSLayoutAttributeFirstBaseline];
    return s;
}

// The menus a window needs to behave: hiding, quitting, and an Edit menu,
// without which Cmd+C and Cmd+V do nothing in a text field.
static void cp_menus(void) {
    NSMenu *bar = [[NSMenu alloc] init];
    NSMenuItem *appItem = [[NSMenuItem alloc] init];
    [bar addItem:appItem];
    NSMenu *appMenu = [[NSMenu alloc] init];
    [appMenu addItemWithTitle:@"Hide" action:@selector(hide:) keyEquivalent:@"h"];
    [appMenu addItemWithTitle:@"Quit" action:@selector(terminate:) keyEquivalent:@"q"];
    [appItem setSubmenu:appMenu];
    NSMenuItem *editItem = [[NSMenuItem alloc] init];
    [bar addItem:editItem];
    NSMenu *edit = [[NSMenu alloc] initWithTitle:@"Edit"];
    [edit addItemWithTitle:@"Cut"        action:@selector(cut:)       keyEquivalent:@"x"];
    [edit addItemWithTitle:@"Copy"       action:@selector(copy:)      keyEquivalent:@"c"];
    [edit addItemWithTitle:@"Paste"      action:@selector(paste:)     keyEquivalent:@"v"];
    [edit addItemWithTitle:@"Select All" action:@selector(selectAll:) keyEquivalent:@"a"];
    [editItem setSubmenu:edit];
    [NSApp setMainMenu:bar];
}

// The list of profiles, with the buttons that add and remove one under it.
static NSStackView *cp_list_column(CPController *c, NSScrollView **outScroll) {
    NSScrollView *lsv = [[NSScrollView alloc] init];
    [lsv setBorderType:NSBezelBorder];
    [lsv setHasVerticalScroller:YES];
    [lsv setAutohidesScrollers:YES];

    c.table = [[NSTableView alloc] init];
    [c.table addTableColumn:[[NSTableColumn alloc] initWithIdentifier:@"name"]];
    [c.table setHeaderView:nil];
    [c.table setAllowsEmptySelection:NO];
    [c.table setRowSizeStyle:NSTableViewRowSizeStyleDefault];
    [c.table setAutoresizingMask:NSViewWidthSizable];
    // The sidebar look AppKit uses for lists like this one, on macOS 11 and
    // later.
    if (@available(macOS 11.0, *))
        [c.table setStyle:NSTableViewStyleSourceList];
    [c.table setDataSource:c];
    [c.table setDelegate:c];
    [lsv setDocumentView:c.table];
    // The rows are drawn inside the column, and the table inside the clip
    // view. Both have to end where the list ends, or the names run out of
    // sight behind the edge.
    [[lsv contentView] setPostsFrameChangedNotifications:YES];
    [[NSNotificationCenter defaultCenter]
        addObserverForName:NSViewFrameDidChangeNotification
            object:[lsv contentView] queue:nil
            usingBlock:^(NSNotification *note) { [c fitColumn]; }];
    // The table must not decide its own column widths behind our back.
    [c.table setColumnAutoresizingStyle:NSTableViewNoColumnAutoresizing];

    c.listButtons = [NSSegmentedControl segmentedControlWithImages:
        @[[NSImage imageNamed:NSImageNameAddTemplate],
          [NSImage imageNamed:NSImageNameRemoveTemplate]]
        trackingMode:NSSegmentSwitchTrackingMomentary
          target:c action:@selector(listAction:)];
    [c.listButtons setSegmentStyle:NSSegmentStyleSmallSquare];

    NSStackView *left = [NSStackView stackViewWithViews:@[lsv, c.listButtons]];
    [left setOrientation:NSUserInterfaceLayoutOrientationVertical];
    [left setAlignment:NSLayoutAttributeLeading];

    *outScroll = lsv;
    return left;
}

static void cp_menubar_item(CPController *c, const void *iconData, int iconLen) {
    NSStatusItem *item = [[NSStatusBar systemStatusBar]
        statusItemWithLength:NSSquareStatusItemLength];
    NSImage *img = [[NSImage alloc]
        initWithData:[NSData dataWithBytes:iconData length:(NSUInteger)iconLen]];
    // The menu bar states its own height; the icon sits inside it.
    CGFloat side = floor([[NSStatusBar systemStatusBar] thickness] * 0.8);
    [img setSize:NSMakeSize(side, side)];
    [img setTemplate:YES];               // follows the menu bar theme
    item.button.image = img;
    item.button.toolTip = @"CC Token Manager";

    NSMenu *menu = [[NSMenu alloc] init];
    NSMenuItem *settings = [[NSMenuItem alloc] initWithTitle:@"Settings…"
        action:@selector(showWindow:) keyEquivalent:@","];
    [settings setTarget:c]; [menu addItem:settings];
    [menu addItem:[NSMenuItem separatorItem]];
    NSMenuItem *quit = [[NSMenuItem alloc] initWithTitle:@"Quit"
        action:@selector(terminate:) keyEquivalent:@"q"];
    [quit setTarget:NSApp]; [menu addItem:quit];
    item.menu = menu;
    c.statusItem = item;
}

void cp_run_settings(const char *profilesJSON, const char *activeID,
                     const char *appliedID, const char *profilesDir,
                     const char *themesJSON, const char *defaultTheme,
                     const char *initialStatus, const void *iconData, int iconLen,
                     int asked) {
    @autoreleasepool {
        cp_profiles_dir = [NSString stringWithUTF8String:profilesDir];
        cp_themes = [NSJSONSerialization JSONObjectWithData:
            [NSData dataWithBytes:themesJSON length:strlen(themesJSON)] options:0 error:nil];
        cp_default_theme = [NSString stringWithUTF8String:defaultTheme];
        [NSApplication sharedApplication];
        CPController *c = [[CPController alloc] init];
        [NSApp setDelegate:c];
        cp_menus();

        // Registers to open at login once and records it, so a removal in System
        // Settings sticks. A failure, as for a bare binary, is tried again at the
        // next launch.
        if (@available(macOS 13.0, *))
            if (!asked && [[SMAppService mainAppService] registerAndReturnError:nil])
                cpSetLaunchAtLogin();

        NSArray *parsed = [NSJSONSerialization JSONObjectWithData:
            [[NSString stringWithUTF8String:profilesJSON] dataUsingEncoding:NSUTF8StringEncoding]
            options:0 error:nil];
        // The window opens on the active profile; the tick goes on it only if
        // the launch applied it.
        NSString *wanted = [NSString stringWithUTF8String:activeID];
        c.appliedID = [NSString stringWithUTF8String:appliedID];

        NSWindow *win = [[NSWindow alloc]
            initWithContentRect:NSZeroRect
                      styleMask:(NSWindowStyleMaskTitled | NSWindowStyleMaskClosable |
                                 NSWindowStyleMaskMiniaturizable | NSWindowStyleMaskResizable)
                        backing:NSBackingStoreBuffered defer:NO];
        [win setTitle:@"CC Token Manager"];
        [win setDelegate:c];
        // Resized by its edges, never blown up to the whole screen: no full
        // screen, and no zoom either.
        [win setCollectionBehavior:NSWindowCollectionBehaviorFullScreenNone];
        [[win standardWindowButton:NSWindowZoomButton] setEnabled:NO];
        c.window = win;
        NSView *cv = [win contentView];

        NSScrollView *lsv = nil;
        NSStackView *left = cp_list_column(c, &lsv);

        // --- right: one form per profile, all in the same place ---
        c.detailHost = [[NSView alloc] init];
        [c.detailHost setContentHuggingPriority:NSLayoutPriorityDefaultLow
                                 forOrientation:NSLayoutConstraintOrientationHorizontal];

        NSStackView *body = [NSStackView stackViewWithViews:@[left, c.detailHost]];
        [body setOrientation:NSUserInterfaceLayoutOrientationHorizontal];
        [body setAlignment:NSLayoutAttributeTop];
        [body setSpacing:[body spacing] * 2];
        [body setTranslatesAutoresizingMaskIntoConstraints:NO];
        [cv addSubview:body];
        [NSLayoutConstraint activateConstraints:@[
            // the margin round the content is the system's to state
            [body.topAnchor     constraintEqualToSystemSpacingBelowAnchor:cv.topAnchor
                                                               multiplier:1],
            [body.leadingAnchor constraintEqualToSystemSpacingAfterAnchor:cv.leadingAnchor
                                                               multiplier:1],
            [cv.trailingAnchor  constraintEqualToSystemSpacingAfterAnchor:body.trailingAnchor
                                                               multiplier:1],
            [cv.bottomAnchor    constraintEqualToSystemSpacingBelowAnchor:body.bottomAnchor
                                                               multiplier:1],
            // the list keeps three tenths of the width, the form takes the rest
            [left.widthAnchor constraintEqualToAnchor:body.widthAnchor multiplier:0.3],
            [lsv.widthAnchor  constraintEqualToAnchor:left.widthAnchor],
            // both columns run to the bottom of the window: the list stretches
            // to meet its buttons, the form to meet its own
            [left.heightAnchor constraintEqualToAnchor:body.heightAnchor],
            [c.detailHost.heightAnchor constraintEqualToAnchor:body.heightAnchor],
        ]];

        c.rows = [NSMutableArray array];
        NSInteger want = 0;
        for (NSUInteger i = 0; i < parsed.count; i++) {
            [c.rows addObject:[c buildDetailFor:parsed[i]]];
            if ([parsed[i][@"id"] isEqual:wanted]) want = i;
        }

        [c reloadListSelecting:want];
        cp_menubar_item(c, iconData, iconLen);

        // Left to itself the window hands the keyboard to whatever comes first,
        // which is the list. The name of the profile on screen is where typing
        // belongs.
        [win setInitialFirstResponder:c.rows[[c.table selectedRow]][@"name"]];

        [win center];
        [cv layoutSubtreeIfNeeded];

        // Left to itself, the app starts in the menu bar only: no window, no Dock
        // icon. A status from startup brings the window up, as Settings… does.
        NSString *startup = [NSString stringWithUTF8String:initialStatus];
        if (startup.length) {
            dispatch_async(dispatch_get_main_queue(), ^{ [c alertText:startup ok:NO]; });
        } else {
            [NSApp setActivationPolicy:NSApplicationActivationPolicyAccessory];
        }
        [NSApp run];
    }
}
