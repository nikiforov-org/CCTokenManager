// The settings window on Linux, in GTK 3, and the icon in the tray, a
// StatusNotifierItem with its menu over D-Bus. It says and does what the macOS
// window does.

#include <gtk/gtk.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>
#include "_cgo_export.h"

#define RS "\x1e"
#define US "\x1f"

// A profile on screen: its row in the list and its form beside it.
typedef struct {
    char *id;
    GtkWidget *row, *tick, *label;
    GtkWidget *page, *name, *token, *dir, *theme, *test, *apply;
    char *secret;     // the token the field stands for while it shows shading
    gboolean shown;   // whether the field shows the token rather than shading
} CPProfile;

static GPtrArray *cp_profiles;
static GtkWidget *cp_window, *cp_list, *cp_stack, *cp_remove;
static char *cp_applied;
static char **cp_theme_values, **cp_theme_names, *cp_default_theme;
static GdkPixbuf *cp_icon;
static gboolean cp_tray;   // whether a tray shows the app's icon
static gboolean cp_quitting;

static void cp_show_window(void);
static void cp_quit(void);

// --- the token's field ---

// A shade for each character of the token: an empty field stays empty.
static char *cp_noise(const char *token) {
    GString *s = g_string_new(NULL);
    for (glong i = g_utf8_strlen(token, -1); i > 0; i--)
        g_string_append(s, "░");
    return g_string_free(s, FALSE);
}

static char *cp_buffer_text(GtkWidget *tv) {
    GtkTextBuffer *b = gtk_text_view_get_buffer(GTK_TEXT_VIEW(tv));
    GtkTextIter s, e;
    gtk_text_buffer_get_bounds(b, &s, &e);
    return gtk_text_buffer_get_text(b, &s, &e, FALSE);
}

static void cp_set_buffer(GtkWidget *tv, const char *text) {
    gtk_text_buffer_set_text(gtk_text_view_get_buffer(GTK_TEXT_VIEW(tv)), text, -1);
}

// cp_token is what the field stands for, shown or not; the caller frees it.
static char *cp_token(CPProfile *p) {
    return p->shown ? cp_buffer_text(p->token) : g_strdup(p->secret);
}

static void cp_show_token(CPProfile *p, gboolean shown) {
    if (shown == p->shown) return;
    if (!shown) {
        g_free(p->secret);
        p->secret = cp_buffer_text(p->token);
    }
    p->shown = shown;
    if (shown)
        cp_set_buffer(p->token, p->secret);
    else {
        char *n = cp_noise(p->secret);
        cp_set_buffer(p->token, n);
        g_free(n);
    }
}

static gboolean cp_token_focus_in(GtkWidget *w, GdkEvent *e, gpointer p) {
    cp_show_token(p, TRUE);
    return FALSE;
}

static gboolean cp_token_focus_out(GtkWidget *w, GdkEvent *e, gpointer p) {
    cp_show_token(p, FALSE);
    return FALSE;
}

// --- dialogs ---

// Every outcome arrives as a dialog. A message over 70 characters is split:
// the first sentence is the headline, the rest the detail.
static void cp_alert(const char *text, gboolean ok) {
    cp_show_window();
    char *head = g_strdup(text);
    const char *detail = "";
    char *dot = strstr(head, ". ");
    if (dot && strlen(text) > 70) {
        dot[1] = 0;
        detail = text + (dot - head) + 2;
    }
    GtkWidget *d = gtk_message_dialog_new(GTK_WINDOW(cp_window), GTK_DIALOG_MODAL,
        ok ? GTK_MESSAGE_INFO : GTK_MESSAGE_WARNING, GTK_BUTTONS_OK, "%s", head);
    if (*detail)
        gtk_message_dialog_format_secondary_text(GTK_MESSAGE_DIALOG(d), "%s", detail);
    gtk_dialog_run(GTK_DIALOG(d));
    gtk_widget_destroy(d);
    g_free(head);
}

// cp_report shows what the Go side answered, "1" or "0" then the message, and
// frees it.
static void cp_report(char *r) {
    cp_alert(r + 1, r[0] == '1');
    free(r);
}

// --- the list ---

static CPProfile *cp_selected(void) {
    GtkListBoxRow *row = gtk_list_box_get_selected_row(GTK_LIST_BOX(cp_list));
    for (guint i = 0; row && i < cp_profiles->len; i++) {
        CPProfile *p = g_ptr_array_index(cp_profiles, i);
        if (p->row == GTK_WIDGET(row)) return p;
    }
    return NULL;
}

// cp_refresh shows each row's name as it stands, and the tick on the applied
// profile.
static void cp_refresh(void) {
    for (guint i = 0; i < cp_profiles->len; i++) {
        CPProfile *p = g_ptr_array_index(cp_profiles, i);
        const char *n = gtk_entry_get_text(GTK_ENTRY(p->name));
        gtk_label_set_text(GTK_LABEL(p->label), *n ? n : "Untitled");
        gboolean applied = *p->id && strcmp(p->id, cp_applied) == 0;
        gtk_label_set_text(GTK_LABEL(p->tick), applied ? "✓" : "");
        gtk_widget_set_tooltip_text(p->row, applied ? "Applied to Claude Code" : NULL);
    }
    gtk_widget_set_sensitive(cp_remove, cp_profiles->len > 1);
}

static void cp_row_selected(GtkListBox *box, GtkListBoxRow *row, gpointer data) {
    CPProfile *p = cp_selected();
    if (!p) return;
    gtk_stack_set_visible_child(GTK_STACK(cp_stack), p->page);
    // Return presses the button the profile on screen offers.
    gtk_window_set_default(GTK_WINDOW(cp_window), p->apply);
}

static void cp_select(guint i) {
    CPProfile *p = g_ptr_array_index(cp_profiles, i);
    gtk_list_box_select_row(GTK_LIST_BOX(cp_list), GTK_LIST_BOX_ROW(p->row));
}

// --- what the buttons do ---

// cp_encode is the profiles as they stand on screen, for the Go side.
static char *cp_encode(void) {
    GString *s = g_string_new(NULL);
    for (guint i = 0; i < cp_profiles->len; i++) {
        CPProfile *p = g_ptr_array_index(cp_profiles, i);
        char *tok = cp_token(p);
        const char *theme = gtk_combo_box_get_active_id(GTK_COMBO_BOX(p->theme));
        g_string_append_printf(s, "%s%s" US "%s" US "%s" US "%s" US "%s", i ? RS : "", p->id,
            gtk_entry_get_text(GTK_ENTRY(p->name)), tok,
            gtk_entry_get_text(GTK_ENTRY(p->dir)), theme ? theme : cp_default_theme);
        g_free(tok);
    }
    return g_string_free(s, FALSE);
}

// The file remembers the applied profile, to apply it at the next launch.
static void cp_save(GtkButton *b, gpointer data) {
    char *all = cp_encode();
    char *r = cpSaveAll(all, cp_applied);
    g_free(all);
    cp_refresh();
    cp_report(r);
}

// Work done off the window's thread: the Go call, and what to do with its answer.
typedef struct {
    char *arg, *id, *result;
    GtkWidget *button;
    gboolean apply;
} CPJob;

static gboolean cp_job_done(gpointer data) {
    CPJob *j = data;
    gtk_widget_set_sensitive(j->button, TRUE);
    g_object_unref(j->button);
    if (j->apply) {
        // A profile that did not go through leaves none applied: ~/.claude is
        // back, and no row gets the tick.
        g_free(cp_applied);
        cp_applied = g_strdup(j->result[0] == '1' ? j->id : "");
        cp_refresh();
    }
    cp_report(j->result);
    g_free(j->arg);
    g_free(j->id);
    g_free(j);
    return G_SOURCE_REMOVE;
}

static gpointer cp_job_run(gpointer data) {
    CPJob *j = data;
    j->result = j->apply ? cpApply(j->arg, j->id) : cpTest(j->arg);
    g_idle_add(cp_job_done, j);
    return NULL;
}

// The disabled button is the only sign of work in progress; the answer comes
// back as a dialog.
static void cp_run_async(GtkWidget *button, char *arg, char *id, gboolean apply) {
    CPJob *j = g_new0(CPJob, 1);
    j->arg = arg;
    j->id = id;
    j->apply = apply;
    j->button = g_object_ref(button);
    gtk_widget_set_sensitive(button, FALSE);
    g_thread_unref(g_thread_new("CCTokenManager", cp_job_run, j));
}

// The token is read from the form, not from the keyring: it may not be saved.
static void cp_test(GtkButton *b, gpointer data) {
    cp_run_async(GTK_WIDGET(b), cp_token(data), NULL, FALSE);
}

// Applying switches Claude Code to the profile as it stands on screen. It does
// not save: the tick shows what is applied now, not what the file remembers.
static void cp_apply(GtkButton *b, gpointer data) {
    CPProfile *p = data;
    cp_run_async(GTK_WIDGET(b), cp_encode(), g_strdup(p->id), TRUE);
}

// The folder is picked the way any folder is, starting where the profile's
// folder is, or would be, or at its nearest parent that exists.
static void cp_choose(GtkButton *b, gpointer data) {
    CPProfile *p = data;
    const char *now = gtk_entry_get_text(GTK_ENTRY(p->dir));
    if (!*now) now = gtk_entry_get_placeholder_text(GTK_ENTRY(p->dir));
    char *start = g_str_has_prefix(now, "~") ? g_build_filename(g_get_home_dir(), now + 1, NULL)
                                             : g_strdup(now);
    while (strlen(start) > 1 && !g_file_test(start, G_FILE_TEST_IS_DIR)) {
        char *up = g_path_get_dirname(start);
        g_free(start);
        start = up;
    }
    GtkWidget *d = gtk_file_chooser_dialog_new("Choose the profile's folder", GTK_WINDOW(cp_window),
        GTK_FILE_CHOOSER_ACTION_SELECT_FOLDER, "_Cancel", GTK_RESPONSE_CANCEL,
        "_Choose", GTK_RESPONSE_ACCEPT, NULL);
    gtk_file_chooser_set_create_folders(GTK_FILE_CHOOSER(d), TRUE);
    gtk_file_chooser_set_current_folder(GTK_FILE_CHOOSER(d), start);
    g_free(start);
    if (gtk_dialog_run(GTK_DIALOG(d)) == GTK_RESPONSE_ACCEPT) {
        char *path = gtk_file_chooser_get_filename(GTK_FILE_CHOOSER(d));
        const char *home = g_get_home_dir();
        size_t n = strlen(home);
        if (strncmp(path, home, n) == 0 && (path[n] == '/' || path[n] == 0)) {
            char *short_ = g_strconcat("~", path + n, NULL);
            gtk_entry_set_text(GTK_ENTRY(p->dir), short_);
            g_free(short_);
        } else
            gtk_entry_set_text(GTK_ENTRY(p->dir), path);
        g_free(path);
    }
    gtk_widget_destroy(d);
}

// The list and the default folder follow the name as it is typed.
static void cp_name_changed(GtkEditable *e, gpointer data) {
    CPProfile *p = data;
    char *dir = cpDefaultDir((char *)gtk_entry_get_text(GTK_ENTRY(p->name)), p->id);
    gtk_entry_set_placeholder_text(GTK_ENTRY(p->dir), dir);
    free(dir);
    cp_refresh();
}

// --- building the window ---

static GtkWidget *cp_label(const char *text) {
    GtkWidget *l = gtk_label_new(text);
    gtk_label_set_xalign(GTK_LABEL(l), 1);
    gtk_style_context_add_class(gtk_widget_get_style_context(l), "dim-label");
    return l;
}

static GtkWidget *cp_field(const char *value) {
    GtkWidget *f = gtk_entry_new();
    gtk_entry_set_text(GTK_ENTRY(f), value);
    gtk_entry_set_activates_default(GTK_ENTRY(f), TRUE);
    gtk_widget_set_hexpand(f, TRUE);
    gtk_style_context_add_class(gtk_widget_get_style_context(f), "cp-mono");
    return f;
}

// cp_token_box is the token's field, four lines of the monospaced font high and
// 48 of its characters wide.
static GtkWidget *cp_token_box(CPProfile *p) {
    GtkWidget *tv = gtk_text_view_new();
    gtk_text_view_set_wrap_mode(GTK_TEXT_VIEW(tv), GTK_WRAP_CHAR);
    gtk_text_view_set_left_margin(GTK_TEXT_VIEW(tv), 4);
    gtk_text_view_set_right_margin(GTK_TEXT_VIEW(tv), 4);
    gtk_text_view_set_top_margin(GTK_TEXT_VIEW(tv), 3);
    gtk_text_view_set_bottom_margin(GTK_TEXT_VIEW(tv), 3);
    gtk_style_context_add_class(gtk_widget_get_style_context(tv), "cp-mono");
    g_signal_connect(tv, "focus-in-event", G_CALLBACK(cp_token_focus_in), p);
    g_signal_connect(tv, "focus-out-event", G_CALLBACK(cp_token_focus_out), p);
    p->token = tv;

    GtkWidget *sw = gtk_scrolled_window_new(NULL, NULL);
    gtk_scrolled_window_set_shadow_type(GTK_SCROLLED_WINDOW(sw), GTK_SHADOW_IN);
    gtk_scrolled_window_set_policy(GTK_SCROLLED_WINDOW(sw), GTK_POLICY_NEVER, GTK_POLICY_AUTOMATIC);
    gtk_container_add(GTK_CONTAINER(sw), tv);
    PangoLayout *l = gtk_widget_create_pango_layout(tv, "0");
    int cw, ch;
    pango_layout_get_pixel_size(l, &cw, &ch);
    g_object_unref(l);
    gtk_widget_set_size_request(sw, cw * 48 + 12, ch * 4 + 10);
    gtk_widget_set_hexpand(sw, TRUE);
    return sw;
}

// cp_build lays out one profile's row and form. Every profile gets its own set,
// so no value is ever shared between two of them.
static CPProfile *cp_build(const char *id, const char *name, const char *token,
                           const char *dir, const char *theme) {
    CPProfile *p = g_new0(CPProfile, 1);
    p->id = g_strdup(id);
    p->secret = g_strdup(token);

    // The row: the mark comes before what it marks, and its place is kept, so
    // the names stay in a column.
    p->tick = gtk_label_new("");
    gtk_label_set_width_chars(GTK_LABEL(p->tick), 2);
    p->label = gtk_label_new(name);
    gtk_label_set_xalign(GTK_LABEL(p->label), 0);
    gtk_label_set_ellipsize(GTK_LABEL(p->label), PANGO_ELLIPSIZE_END);
    GtkWidget *rb = gtk_box_new(GTK_ORIENTATION_HORIZONTAL, 4);
    gtk_widget_set_margin_top(rb, 6);
    gtk_widget_set_margin_bottom(rb, 6);
    gtk_widget_set_margin_end(rb, 6);
    gtk_box_pack_start(GTK_BOX(rb), p->tick, FALSE, FALSE, 0);
    gtk_box_pack_start(GTK_BOX(rb), p->label, TRUE, TRUE, 0);
    p->row = gtk_list_box_row_new();
    gtk_container_add(GTK_CONTAINER(p->row), rb);
    gtk_container_add(GTK_CONTAINER(cp_list), p->row);

    // The form.
    p->name = cp_field(name);
    GtkWidget *tokenBox = cp_token_box(p);
    char *n = cp_noise(token);
    cp_set_buffer(p->token, n);
    g_free(n);
    p->test = gtk_button_new_with_label("Test connection");
    gtk_widget_set_halign(p->test, GTK_ALIGN_START);
    g_signal_connect(p->test, "clicked", G_CALLBACK(cp_test), p);
    p->dir = cp_field(dir);
    char *def = cpDefaultDir((char *)name, (char *)id);
    gtk_entry_set_placeholder_text(GTK_ENTRY(p->dir), def);
    free(def);
    GtkWidget *choose = gtk_button_new_with_label("Choose…");
    g_signal_connect(choose, "clicked", G_CALLBACK(cp_choose), p);
    GtkWidget *dirRow = gtk_box_new(GTK_ORIENTATION_HORIZONTAL, 6);
    gtk_box_pack_start(GTK_BOX(dirRow), p->dir, TRUE, TRUE, 0);
    gtk_box_pack_start(GTK_BOX(dirRow), choose, FALSE, FALSE, 0);
    p->theme = gtk_combo_box_text_new();
    for (int i = 0; cp_theme_values[i]; i++)
        gtk_combo_box_text_append(GTK_COMBO_BOX_TEXT(p->theme), cp_theme_values[i], cp_theme_names[i]);
    if (!gtk_combo_box_set_active_id(GTK_COMBO_BOX(p->theme), theme))
        gtk_combo_box_set_active_id(GTK_COMBO_BOX(p->theme), cp_default_theme);
    g_signal_connect(p->name, "changed", G_CALLBACK(cp_name_changed), p);

    GtkWidget *g = gtk_grid_new();
    gtk_grid_set_row_spacing(GTK_GRID(g), 8);
    gtk_grid_set_column_spacing(GTK_GRID(g), 8);
    GtkWidget *tokenLabel = cp_label("OAuth token");
    gtk_widget_set_valign(tokenLabel, GTK_ALIGN_START);
    gtk_widget_set_margin_top(tokenLabel, 4);
    gtk_grid_attach(GTK_GRID(g), cp_label("Profile name"), 0, 0, 1, 1);
    gtk_grid_attach(GTK_GRID(g), p->name, 1, 0, 1, 1);
    gtk_grid_attach(GTK_GRID(g), tokenLabel, 0, 1, 1, 1);
    gtk_grid_attach(GTK_GRID(g), tokenBox, 1, 1, 1, 1);
    gtk_grid_attach(GTK_GRID(g), p->test, 1, 2, 1, 1);
    gtk_grid_attach(GTK_GRID(g), cp_label("Profile folder"), 0, 3, 1, 1);
    gtk_grid_attach(GTK_GRID(g), dirRow, 1, 3, 1, 1);
    gtk_grid_attach(GTK_GRID(g), cp_label("Profile theme"), 0, 4, 1, 1);
    gtk_grid_attach(GTK_GRID(g), p->theme, 1, 4, 1, 1);

    // Acting on the whole profile rather than on one field, these two stand
    // apart from the form's rows, in its bottom corner.
    GtkWidget *save = gtk_button_new_with_label("Save");
    g_signal_connect(save, "clicked", G_CALLBACK(cp_save), p);
    p->apply = gtk_button_new_with_label("Apply profile");
    gtk_widget_set_can_default(p->apply, TRUE);
    g_signal_connect(p->apply, "clicked", G_CALLBACK(cp_apply), p);
    GtkWidget *actions = gtk_box_new(GTK_ORIENTATION_HORIZONTAL, 8);
    gtk_widget_set_halign(actions, GTK_ALIGN_END);
    gtk_widget_set_valign(actions, GTK_ALIGN_END);
    gtk_widget_set_vexpand(actions, TRUE);
    gtk_widget_set_margin_top(actions, 32);
    gtk_box_pack_start(GTK_BOX(actions), save, FALSE, FALSE, 0);
    gtk_box_pack_start(GTK_BOX(actions), p->apply, FALSE, FALSE, 0);

    p->page = gtk_box_new(GTK_ORIENTATION_VERTICAL, 0);
    gtk_box_pack_start(GTK_BOX(p->page), g, FALSE, FALSE, 0);
    gtk_box_pack_start(GTK_BOX(p->page), actions, TRUE, TRUE, 0);
    gtk_stack_add_named(GTK_STACK(cp_stack), p->page, p->id);
    gtk_widget_show_all(p->row);
    gtk_widget_show_all(p->page);
    g_ptr_array_add(cp_profiles, p);
    return p;
}

// The buttons under the list: + adds a profile, − deletes the one on screen.
static void cp_add(GtkButton *b, gpointer data) {
    char *uuid = g_uuid_string_random();
    char *id = g_ascii_strup(uuid, -1);
    CPProfile *p = cp_build(id, "New profile", "", "", cp_default_theme);
    g_free(uuid);
    g_free(id);
    cp_select(cp_profiles->len - 1);
    cp_refresh();
    gtk_widget_grab_focus(p->name);
}

static void cp_delete(GtkButton *b, gpointer data) {
    CPProfile *p = cp_selected();
    if (!p) return;
    GtkWidget *d = gtk_message_dialog_new(GTK_WINDOW(cp_window), GTK_DIALOG_MODAL,
        GTK_MESSAGE_QUESTION, GTK_BUTTONS_NONE, "Delete profile “%s”?",
        gtk_entry_get_text(GTK_ENTRY(p->name)));
    gtk_message_dialog_format_secondary_text(GTK_MESSAGE_DIALOG(d), "Its token is removed from this app.");
    gtk_dialog_add_buttons(GTK_DIALOG(d), "Cancel", GTK_RESPONSE_CANCEL, "Delete", GTK_RESPONSE_ACCEPT, NULL);
    gint answer = gtk_dialog_run(GTK_DIALOG(d));
    gtk_widget_destroy(d);
    if (answer != GTK_RESPONSE_ACCEPT) return;

    guint i = 0;
    while (g_ptr_array_index(cp_profiles, i) != p) i++;
    // The tick cannot stay on a profile that is gone, nor move to one that is
    // not applied.
    if (strcmp(p->id, cp_applied) == 0) {
        g_free(cp_applied);
        cp_applied = g_strdup("");
    }
    g_ptr_array_remove_index(cp_profiles, i);
    gtk_widget_destroy(p->row);
    gtk_widget_destroy(p->page);
    g_free(p->id);
    g_free(p->secret);
    g_free(p);
    cp_select(MIN(i, cp_profiles->len - 1));
    cp_refresh();
}

// --- the window and the app ---

static void cp_show_window(void) {
    gtk_window_present(GTK_WINDOW(cp_window));
}

// Closing hides the window when the tray can bring it back, and minimises it
// when there is no tray.
static gboolean cp_close(GtkWidget *w, GdkEvent *e, gpointer data) {
    if (cp_tray)
        gtk_widget_hide(w);
    else
        gtk_window_iconify(GTK_WINDOW(w));
    return TRUE;
}

// Quitting takes the profile back and leaves at once, whatever dialog is open:
// one running its own loop would otherwise hold the app up.
static void cp_quit(void) {
    if (cp_quitting) return;
    cp_quitting = TRUE;
    cpRevert();
    exit(0);
}

// --- the tray: a StatusNotifierItem, and its menu as com.canonical.dbusmenu ---

static const char *cp_sni_xml =
    "<node><interface name='org.kde.StatusNotifierItem'>"
    "<property name='Category' type='s' access='read'/>"
    "<property name='Id' type='s' access='read'/>"
    "<property name='Title' type='s' access='read'/>"
    "<property name='Status' type='s' access='read'/>"
    "<property name='IconName' type='s' access='read'/>"
    "<property name='IconPixmap' type='a(iiay)' access='read'/>"
    "<property name='ToolTip' type='(sa(iiay)ss)' access='read'/>"
    "<property name='ItemIsMenu' type='b' access='read'/>"
    "<property name='Menu' type='o' access='read'/>"
    "<method name='Activate'><arg type='i' direction='in'/><arg type='i' direction='in'/></method>"
    "<method name='SecondaryActivate'><arg type='i' direction='in'/><arg type='i' direction='in'/></method>"
    "<method name='ContextMenu'><arg type='i' direction='in'/><arg type='i' direction='in'/></method>"
    "<method name='Scroll'><arg type='i' direction='in'/><arg type='s' direction='in'/></method>"
    "</interface></node>";

static const char *cp_menu_xml =
    "<node><interface name='com.canonical.dbusmenu'>"
    "<property name='Version' type='u' access='read'/>"
    "<property name='TextDirection' type='s' access='read'/>"
    "<property name='Status' type='s' access='read'/>"
    "<property name='IconThemePath' type='as' access='read'/>"
    "<method name='GetLayout'><arg type='i' direction='in'/><arg type='i' direction='in'/>"
    "<arg type='as' direction='in'/><arg type='u' direction='out'/><arg type='(ia{sv}av)' direction='out'/></method>"
    "<method name='GetGroupProperties'><arg type='ai' direction='in'/><arg type='as' direction='in'/>"
    "<arg type='a(ia{sv})' direction='out'/></method>"
    "<method name='GetProperty'><arg type='i' direction='in'/><arg type='s' direction='in'/>"
    "<arg type='v' direction='out'/></method>"
    "<method name='Event'><arg type='i' direction='in'/><arg type='s' direction='in'/>"
    "<arg type='v' direction='in'/><arg type='u' direction='in'/></method>"
    "<method name='EventGroup'><arg type='a(isvu)' direction='in'/><arg type='ai' direction='out'/></method>"
    "<method name='AboutToShow'><arg type='i' direction='in'/><arg type='b' direction='out'/></method>"
    "<method name='AboutToShowGroup'><arg type='ai' direction='in'/><arg type='ai' direction='out'/>"
    "<arg type='ai' direction='out'/></method>"
    "<signal name='LayoutUpdated'><arg type='u'/><arg type='i'/></signal>"
    "</interface></node>";

// The menu: 1 Settings…, 2 a separator, 3 Quit, under the root 0.
static GVariant *cp_menu_props(int id) {
    GVariantBuilder b;
    g_variant_builder_init(&b, G_VARIANT_TYPE("a{sv}"));
    if (id == 0)
        g_variant_builder_add(&b, "{sv}", "children-display", g_variant_new_string("submenu"));
    else if (id == 2)
        g_variant_builder_add(&b, "{sv}", "type", g_variant_new_string("separator"));
    else
        g_variant_builder_add(&b, "{sv}", "label", g_variant_new_string(id == 1 ? "Settings…" : "Quit"));
    return g_variant_builder_end(&b);
}

static GVariant *cp_menu_item(int id, gboolean children) {
    GVariantBuilder ch;
    g_variant_builder_init(&ch, G_VARIANT_TYPE("av"));
    if (id == 0 && children)
        for (int i = 1; i <= 3; i++)
            g_variant_builder_add(&ch, "v", cp_menu_item(i, FALSE));
    return g_variant_new("(i@a{sv}av)", id, cp_menu_props(id), &ch);
}

static gboolean cp_menu_clicked(gpointer data) {
    if (GPOINTER_TO_INT(data) == 1)
        cp_show_window();
    else
        cp_quit();
    return G_SOURCE_REMOVE;
}

static void cp_menu_event(int id, const char *event) {
    if (strcmp(event, "clicked") == 0 && (id == 1 || id == 3))
        g_idle_add(cp_menu_clicked, GINT_TO_POINTER(id));
}

static void cp_menu_call(GDBusConnection *c, const char *sender, const char *path,
                         const char *iface, const char *method, GVariant *args,
                         GDBusMethodInvocation *inv, gpointer data) {
    if (strcmp(method, "GetLayout") == 0) {
        gint parent, depth;
        g_variant_get(args, "(ii@as)", &parent, &depth, NULL);
        g_dbus_method_invocation_return_value(inv,
            g_variant_new("(u@(ia{sv}av))", 1, cp_menu_item(parent, depth != 0)));
    } else if (strcmp(method, "GetGroupProperties") == 0) {
        GVariantIter *ids;
        gint id;
        GVariantBuilder b;
        g_variant_builder_init(&b, G_VARIANT_TYPE("a(ia{sv})"));
        g_variant_get(args, "(ai@as)", &ids, NULL);
        while (g_variant_iter_next(ids, "i", &id))
            if (id >= 0 && id <= 3)
                g_variant_builder_add(&b, "(i@a{sv})", id, cp_menu_props(id));
        g_variant_iter_free(ids);
        g_dbus_method_invocation_return_value(inv, g_variant_new("(a(ia{sv}))", &b));
    } else if (strcmp(method, "GetProperty") == 0) {
        gint id;
        const char *name;
        g_variant_get(args, "(i&s)", &id, &name);
        GVariant *props = cp_menu_props(id);
        GVariant *v = g_variant_lookup_value(props, name, NULL);
        g_variant_unref(g_variant_ref_sink(props));
        g_dbus_method_invocation_return_value(inv, g_variant_new("(v)", v ? v : g_variant_new_string("")));
        if (v) g_variant_unref(v);
    } else if (strcmp(method, "Event") == 0) {
        gint id;
        const char *event;
        g_variant_get(args, "(i&svu)", &id, &event, NULL, NULL);
        cp_menu_event(id, event);
        g_dbus_method_invocation_return_value(inv, NULL);
    } else if (strcmp(method, "EventGroup") == 0) {
        GVariantIter *events;
        gint id;
        const char *event;
        g_variant_get(args, "(a(isvu))", &events);
        while (g_variant_iter_next(events, "(i&svu)", &id, &event, NULL, NULL))
            cp_menu_event(id, event);
        g_variant_iter_free(events);
        g_dbus_method_invocation_return_value(inv, g_variant_new("(@ai)", g_variant_new_array(G_VARIANT_TYPE_INT32, NULL, 0)));
    } else if (strcmp(method, "AboutToShow") == 0) {
        g_dbus_method_invocation_return_value(inv, g_variant_new("(b)", FALSE));
    } else if (strcmp(method, "AboutToShowGroup") == 0) {
        g_dbus_method_invocation_return_value(inv, g_variant_new("(@ai@ai)", g_variant_new_array(G_VARIANT_TYPE_INT32, NULL, 0),
                                                                 g_variant_new_array(G_VARIANT_TYPE_INT32, NULL, 0)));
    } else
        g_dbus_method_invocation_return_dbus_error(inv, "org.freedesktop.DBus.Error.UnknownMethod", method);
}

static GVariant *cp_menu_prop(GDBusConnection *c, const char *sender, const char *path,
                              const char *iface, const char *prop, GError **err, gpointer data) {
    if (strcmp(prop, "Version") == 0) return g_variant_new_uint32(3);
    if (strcmp(prop, "TextDirection") == 0) return g_variant_new_string("ltr");
    if (strcmp(prop, "Status") == 0) return g_variant_new_string("normal");
    return g_variant_new_strv(NULL, 0);
}

// cp_pixmaps is the app's icon at the sizes a tray asks for, as ARGB in network
// byte order.
static GVariant *cp_pixmaps(void) {
    GVariantBuilder b;
    g_variant_builder_init(&b, G_VARIANT_TYPE("a(iiay)"));
    int sizes[] = {16, 22, 24, 32, 48, 64};
    for (unsigned k = 0; k < G_N_ELEMENTS(sizes); k++) {
        int s = sizes[k];
        GdkPixbuf *pb = gdk_pixbuf_scale_simple(cp_icon, s, s, GDK_INTERP_HYPER);
        int stride = gdk_pixbuf_get_rowstride(pb), ch = gdk_pixbuf_get_n_channels(pb);
        const guchar *px = gdk_pixbuf_read_pixels(pb);
        guchar *argb = g_malloc(s * s * 4);
        for (int y = 0; y < s; y++)
            for (int x = 0; x < s; x++) {
                const guchar *q = px + y * stride + x * ch;
                guchar *o = argb + (y * s + x) * 4;
                o[0] = ch == 4 ? q[3] : 255;
                o[1] = q[0];
                o[2] = q[1];
                o[3] = q[2];
            }
        g_variant_builder_add(&b, "(ii@ay)", s, s,
            g_variant_new_fixed_array(G_VARIANT_TYPE_BYTE, argb, s * s * 4, 1));
        g_free(argb);
        g_object_unref(pb);
    }
    return g_variant_builder_end(&b);
}

static gboolean cp_activated(gpointer data) {
    cp_show_window();
    return G_SOURCE_REMOVE;
}

static void cp_sni_call(GDBusConnection *c, const char *sender, const char *path,
                        const char *iface, const char *method, GVariant *args,
                        GDBusMethodInvocation *inv, gpointer data) {
    if (strcmp(method, "Activate") == 0)
        g_idle_add(cp_activated, NULL);
    g_dbus_method_invocation_return_value(inv, NULL);
}

static GVariant *cp_sni_prop(GDBusConnection *c, const char *sender, const char *path,
                             const char *iface, const char *prop, GError **err, gpointer data) {
    if (strcmp(prop, "Category") == 0) return g_variant_new_string("ApplicationStatus");
    if (strcmp(prop, "Id") == 0) return g_variant_new_string("CCTokenManager");
    if (strcmp(prop, "Title") == 0) return g_variant_new_string("CC Token Manager");
    if (strcmp(prop, "Status") == 0) return g_variant_new_string("Active");
    if (strcmp(prop, "IconName") == 0) return g_variant_new_string("");
    if (strcmp(prop, "IconPixmap") == 0) return cp_pixmaps();
    if (strcmp(prop, "ToolTip") == 0)
        return g_variant_new("(s@a(iiay)ss)", "", g_variant_new_array(G_VARIANT_TYPE("(iiay)"), NULL, 0),
                             "CC Token Manager", "");
    if (strcmp(prop, "ItemIsMenu") == 0) return g_variant_new_boolean(FALSE);
    if (strcmp(prop, "Menu") == 0) return g_variant_new_object_path("/MenuBar");
    return NULL;
}

static GDBusConnection *cp_bus;
static char *cp_bus_name;

// cp_register asks the tray, if there is one, to show the item.
static gboolean cp_register(void) {
    GVariant *r = g_dbus_connection_call_sync(cp_bus, "org.kde.StatusNotifierWatcher",
        "/StatusNotifierWatcher", "org.kde.StatusNotifierWatcher", "RegisterStatusNotifierItem",
        g_variant_new("(s)", cp_bus_name), NULL, G_DBUS_CALL_FLAGS_NONE, 2000, NULL, NULL);
    if (r) g_variant_unref(r);
    return r != NULL;
}

static void cp_watcher_appeared(GDBusConnection *c, const char *name, const char *owner, gpointer d) {
    cp_tray = cp_register();
}

// A tray that goes away leaves the window as the way in.
static void cp_watcher_vanished(GDBusConnection *c, const char *name, gpointer d) {
    if (cp_tray && !gtk_widget_get_visible(cp_window)) {
        gtk_widget_show(cp_window);
        gtk_window_iconify(GTK_WINDOW(cp_window));
    }
    cp_tray = FALSE;
}

static void cp_start_tray(void) {
    cp_bus = g_bus_get_sync(G_BUS_TYPE_SESSION, NULL, NULL);
    if (!cp_bus) return;
    static const GDBusInterfaceVTable sni = {cp_sni_call, cp_sni_prop, NULL};
    static const GDBusInterfaceVTable menu = {cp_menu_call, cp_menu_prop, NULL};
    GDBusNodeInfo *sn = g_dbus_node_info_new_for_xml(cp_sni_xml, NULL);
    GDBusNodeInfo *mn = g_dbus_node_info_new_for_xml(cp_menu_xml, NULL);
    g_dbus_connection_register_object(cp_bus, "/StatusNotifierItem", sn->interfaces[0], &sni, NULL, NULL, NULL);
    g_dbus_connection_register_object(cp_bus, "/MenuBar", mn->interfaces[0], &menu, NULL, NULL, NULL);
    cp_bus_name = g_strdup_printf("org.kde.StatusNotifierItem-%d-1", getpid());
    g_bus_own_name_on_connection(cp_bus, cp_bus_name, G_BUS_NAME_OWNER_FLAGS_NONE, NULL, NULL, NULL, NULL);
    cp_tray = cp_register();
    g_bus_watch_name_on_connection(cp_bus, "org.kde.StatusNotifierWatcher", G_BUS_NAME_WATCHER_FLAGS_NONE,
        cp_watcher_appeared, cp_watcher_vanished, NULL, NULL);
}

// cp_register_login adds the app to what the desktop starts at login, once: a
// removal in the system's settings then stays.
static void cp_register_login(void) {
    char *exe = g_file_read_link("/proc/self/exe", NULL);
    if (!exe) return;
    char *dir = g_build_filename(g_get_user_config_dir(), "autostart", NULL);
    char *file = g_build_filename(dir, "org.nikiforov.CCTokenManager.desktop", NULL);
    GString *quoted = g_string_new("\"");
    for (const char *c = exe; *c; c++) {
        if (strchr("\"`$\\", *c)) g_string_append(quoted, "\\\\");
        g_string_append_c(quoted, *c);
    }
    g_string_append_c(quoted, '"');
    char *entry = g_strdup_printf("[Desktop Entry]\nType=Application\nName=CC Token Manager\n"
                                  "Exec=%s\nIcon=CCTokenManager\nX-GNOME-Autostart-enabled=true\n",
                                  quoted->str);
    if (g_mkdir_with_parents(dir, 0700) == 0 && g_file_set_contents(file, entry, -1, NULL))
        cpSetLaunchAtLogin();
    g_free(entry);
    g_string_free(quoted, TRUE);
    g_free(file);
    g_free(dir);
    g_free(exe);
}

void cp_run_settings(const char *profiles, const char *activeID, const char *appliedID,
                     const char *themes, const char *defaultTheme, const char *initialStatus,
                     const void *iconData, int iconLen, int asked) {
    gtk_init(NULL, NULL);
    g_set_prgname("CCTokenManager");
    g_set_application_name("CC Token Manager");

    GdkPixbufLoader *ld = gdk_pixbuf_loader_new();
    gdk_pixbuf_loader_write(ld, iconData, iconLen, NULL);
    gdk_pixbuf_loader_close(ld, NULL);
    cp_icon = g_object_ref(gdk_pixbuf_loader_get_pixbuf(ld));
    g_object_unref(ld);

    GtkCssProvider *css = gtk_css_provider_new();
    gtk_css_provider_load_from_data(css, ".cp-mono { font-family: monospace; }", -1, NULL);
    gtk_style_context_add_provider_for_screen(gdk_screen_get_default(), GTK_STYLE_PROVIDER(css),
                                              GTK_STYLE_PROVIDER_PRIORITY_APPLICATION);

    char **recs = g_strsplit(themes, RS, -1);
    guint n = g_strv_length(recs);
    cp_theme_values = g_new0(char *, n + 1);
    cp_theme_names = g_new0(char *, n + 1);
    for (guint i = 0; i < n; i++) {
        char **f = g_strsplit(recs[i], US, 2);
        cp_theme_values[i] = g_strdup(f[0]);
        cp_theme_names[i] = g_strdup(f[1] ? f[1] : f[0]);
        g_strfreev(f);
    }
    g_strfreev(recs);
    cp_default_theme = g_strdup(defaultTheme);
    cp_applied = g_strdup(appliedID);
    cp_profiles = g_ptr_array_new();

    if (!asked) cp_register_login();

    cp_window = gtk_window_new(GTK_WINDOW_TOPLEVEL);
    gtk_window_set_title(GTK_WINDOW(cp_window), "CC Token Manager");
    gtk_window_set_icon(GTK_WINDOW(cp_window), cp_icon);
    gtk_window_set_position(GTK_WINDOW(cp_window), GTK_WIN_POS_CENTER);
    g_signal_connect(cp_window, "delete-event", G_CALLBACK(cp_close), NULL);

    // The list of profiles, with the buttons that add and remove one under it.
    cp_list = gtk_list_box_new();
    gtk_list_box_set_selection_mode(GTK_LIST_BOX(cp_list), GTK_SELECTION_BROWSE);
    g_signal_connect(cp_list, "row-selected", G_CALLBACK(cp_row_selected), NULL);
    GtkWidget *lsv = gtk_scrolled_window_new(NULL, NULL);
    gtk_scrolled_window_set_shadow_type(GTK_SCROLLED_WINDOW(lsv), GTK_SHADOW_IN);
    gtk_scrolled_window_set_policy(GTK_SCROLLED_WINDOW(lsv), GTK_POLICY_NEVER, GTK_POLICY_AUTOMATIC);
    gtk_widget_set_vexpand(lsv, TRUE);
    gtk_container_add(GTK_CONTAINER(lsv), cp_list);
    GtkWidget *add = gtk_button_new_from_icon_name("list-add-symbolic", GTK_ICON_SIZE_BUTTON);
    cp_remove = gtk_button_new_from_icon_name("list-remove-symbolic", GTK_ICON_SIZE_BUTTON);
    g_signal_connect(add, "clicked", G_CALLBACK(cp_add), NULL);
    g_signal_connect(cp_remove, "clicked", G_CALLBACK(cp_delete), NULL);
    GtkWidget *buttons = gtk_box_new(GTK_ORIENTATION_HORIZONTAL, 0);
    gtk_style_context_add_class(gtk_widget_get_style_context(buttons), "linked");
    gtk_box_pack_start(GTK_BOX(buttons), add, FALSE, FALSE, 0);
    gtk_box_pack_start(GTK_BOX(buttons), cp_remove, FALSE, FALSE, 0);
    GtkWidget *left = gtk_box_new(GTK_ORIENTATION_VERTICAL, 6);
    gtk_box_pack_start(GTK_BOX(left), lsv, TRUE, TRUE, 0);
    gtk_box_pack_start(GTK_BOX(left), buttons, FALSE, FALSE, 0);

    // One form per profile, all in the same place; the list keeps three tenths
    // of the width, the form takes the rest.
    cp_stack = gtk_stack_new();
    GtkWidget *body = gtk_grid_new();
    gtk_grid_set_column_homogeneous(GTK_GRID(body), TRUE);
    gtk_grid_set_column_spacing(GTK_GRID(body), 18);
    gtk_container_set_border_width(GTK_CONTAINER(body), 12);
    gtk_grid_attach(GTK_GRID(body), left, 0, 0, 3, 1);
    gtk_grid_attach(GTK_GRID(body), cp_stack, 3, 0, 7, 1);
    gtk_container_add(GTK_CONTAINER(cp_window), body);

    guint want = 0;
    char **ps = g_strsplit(profiles, RS, -1);
    for (guint i = 0; ps[i]; i++) {
        char **f = g_strsplit(ps[i], US, 5);
        if (g_strv_length(f) == 5) {
            cp_build(f[0], f[1], f[2], f[3], f[4]);
            if (strcmp(f[0], activeID) == 0) want = cp_profiles->len - 1;
        }
        g_strfreev(f);
    }
    g_strfreev(ps);
    gtk_widget_show_all(body);
    cp_select(want);
    cp_refresh();
    // The name of the profile on screen is where typing belongs.
    gtk_widget_grab_focus(((CPProfile *)g_ptr_array_index(cp_profiles, want))->name);

    cp_start_tray();
    // Left to itself, the app starts in the tray only; with no tray, or a status
    // from startup to say, the window comes up.
    if (*initialStatus) {
        char *s = g_strdup(initialStatus);
        cp_alert(s, FALSE);
        g_free(s);
    } else if (!cp_tray)
        gtk_widget_show(cp_window);
    gtk_main();
}
