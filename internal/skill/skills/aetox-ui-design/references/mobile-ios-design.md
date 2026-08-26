
# iOS Mobile Design

Master iOS Human Interface Guidelines (HIG) and SwiftUI patterns to build polished, native iOS applications that feel at home on Apple platforms.

## When to Use This Skill

- Designing iOS app interfaces following Apple HIG
- Building SwiftUI views and layouts
- Implementing iOS navigation patterns (NavigationStack, TabView, sheets)
- Creating adaptive layouts for iPhone and iPad
- Using SF Symbols and system typography
- Building accessible iOS interfaces
- Implementing iOS-specific gestures and interactions
- Designing for Dynamic Type and Dark Mode

## Core Concepts

### 1. Human Interface Guidelines Principles

**Clarity**: Content is legible, icons are precise, adornments are subtle
**Deference**: UI helps users understand content without competing with it
**Depth**: Visual layers and motion convey hierarchy and enable navigation

**Platform Considerations:**

- **iOS**: Touch-first, compact displays, portrait orientation
- **iPadOS**: Larger canvas, multitasking, pointer support
- **visionOS**: Spatial computing, eye/hand input

### 2. SwiftUI Layout System

**Stack-Based Layouts:**

```swift
// Vertical stack with alignment
VStack(alignment: .leading, spacing: 12) {
    Text("Title")
        .font(.headline)
    Text("Subtitle")
        .font(.subheadline)
        .foregroundStyle(.secondary)
}

// Horizontal stack with flexible spacing
HStack {
    Image(systemName: "star.fill")
    Text("Featured")
    Spacer()
    Text("View All")
        .foregroundStyle(.blue)
}
```

**Grid Layouts:**

```swift
// Adaptive grid that fills available width
LazyVGrid(columns: [
    GridItem(.adaptive(minimum: 150, maximum: 200))
], spacing: 16) {
    ForEach(items) { item in
        ItemCard(item: item)
    }
}

// Fixed column grid
LazyVGrid(columns: [
    GridItem(.flexible()),
    GridItem(.flexible()),
    GridItem(.flexible())
], spacing: 12) {
    ForEach(items) { item in
        ItemThumbnail(item: item)
    }
}
```

### 3. Navigation Patterns

**NavigationStack (iOS 16+):**

```swift
struct ContentView: View {
    @State private var path = NavigationPath()

    var body: some View {
        NavigationStack(path: $path) {
            List(items) { item in
                NavigationLink(value: item) {
                    ItemRow(item: item)
                }
            }
            .navigationTitle("Items")
            .navigationDestination(for: Item.self) { item in
                ItemDetailView(item: item)
            }
        }
    }
}
```

**TabView (iOS 18+):**

```swift
struct MainTabView: View {
    @State private var selectedTab = 0

    var body: some View {
        TabView(selection: $selectedTab) {
            Tab("Home", systemImage: "house", value: 0) {
                HomeView()
            }

            Tab("Search", systemImage: "magnifyingglass", value: 1) {
                SearchView()
            }

            Tab("Profile", systemImage: "person", value: 2) {
                ProfileView()
            }
        }
    }
}
```

### 4. System Integration

**SF Symbols:**

```swift
// Basic symbol
Image(systemName: "heart.fill")
    .foregroundStyle(.red)

// Symbol with rendering mode
Image(systemName: "cloud.sun.fill")
    .symbolRenderingMode(.multicolor)

// Variable symbol (iOS 16+)
Image(systemName: "speaker.wave.3.fill", variableValue: volume)

// Symbol effect (iOS 17+)
Image(systemName: "bell.fill")
    .symbolEffect(.bounce, value: notificationCount)
```

**Dynamic Type:**

```swift
// Use semantic fonts
Text("Headline")
    .font(.headline)

Text("Body text that scales with user preferences")
    .font(.body)

// Custom font that respects Dynamic Type
Text("Custom")
    .font(.custom("Avenir", size: 17, relativeTo: .body))
```

### 5. Visual Design

**Colors and Materials:**

```swift
// Semantic colors that adapt to light/dark mode
Text("Primary")
    .foregroundStyle(.primary)
Text("Secondary")
    .foregroundStyle(.secondary)

// System materials for blur effects
Rectangle()
    .fill(.ultraThinMaterial)
    .frame(height: 100)

// Vibrant materials for overlays
Text("Overlay")
    .padding()
    .background(.regularMaterial, in: RoundedRectangle(cornerRadius: 12))
```

**Shadows and Depth:**

```swift
// Standard card shadow
RoundedRectangle(cornerRadius: 16)
    .fill(.background)
    .shadow(color: .black.opacity(0.1), radius: 8, y: 4)

// Elevated appearance
.shadow(radius: 2, y: 1)
.shadow(radius: 8, y: 4)
```

## Quick Start Component

```swift
import SwiftUI

struct FeatureCard: View {
    let title: String
    let description: String
    let systemImage: String

    var body: some View {
        HStack(spacing: 16) {
            Image(systemName: systemImage)
                .font(.title)
                .foregroundStyle(.blue)
                .frame(width: 44, height: 44)
                .background(.blue.opacity(0.1), in: Circle())

            VStack(alignment: .leading, spacing: 4) {
                Text(title)
                    .font(.headline)
                Text(description)
                    .font(.subheadline)
                    .foregroundStyle(.secondary)
                    .lineLimit(2)
            }

            Spacer()

            Image(systemName: "chevron.right")
                .foregroundStyle(.tertiary)
        }
        .padding()
        .background(.background, in: RoundedRectangle(cornerRadius: 12))
        .shadow(color: .black.opacity(0.05), radius: 4, y: 2)
    }
}
```

## Best Practices

1. **Use Semantic Colors**: Always use `.primary`, `.secondary`, `.background` for automatic light/dark mode support
2. **Embrace SF Symbols**: Use system symbols for consistency and automatic accessibility
3. **Support Dynamic Type**: Use semantic fonts (`.body`, `.headline`) instead of fixed sizes
4. **Add Accessibility**: Include `.accessibilityLabel()` and `.accessibilityHint()` modifiers
5. **Use Safe Areas**: Respect `safeAreaInset` and avoid hardcoded padding at screen edges
6. **Implement State Restoration**: Use `@SceneStorage` for preserving user state
7. **Support iPad Multitasking**: Design for split view and slide over
8. **Test on Device**: Simulator doesn't capture full haptic and performance experience

## Common Issues

- **Layout Breaking**: Use `.fixedSize()` sparingly; prefer flexible layouts
- **Performance Issues**: Use `LazyVStack`/`LazyHStack` for long scrolling lists
- **Navigation Bugs**: Ensure `NavigationLink` values are `Hashable`
- **Dark Mode Problems**: Avoid hardcoded colors; use semantic or asset catalog colors
- **Accessibility Failures**: Test with VoiceOver enabled
- **Memory Leaks**: Watch for strong reference cycles in closures


---

## อ้างอิง: hig-patterns

# iOS Human Interface Guidelines Patterns

## Layout and Spacing

### Standard Margins and Padding

```swift
// System standard margins
private let standardMargin: CGFloat = 16
private let compactMargin: CGFloat = 8
private let largeMargin: CGFloat = 24

// Content insets following HIG
extension EdgeInsets {
    static let standard = EdgeInsets(top: 16, leading: 16, bottom: 16, trailing: 16)
    static let listRow = EdgeInsets(top: 12, leading: 16, bottom: 12, trailing: 16)
    static let card = EdgeInsets(top: 16, leading: 16, bottom: 16, trailing: 16)
}
```

### Safe Area Handling

```swift
struct SafeAreaAwareView: View {
    var body: some View {
        ScrollView {
            LazyVStack(spacing: 16) {
                ForEach(items) { item in
                    ItemRow(item: item)
                }
            }
            .padding(.horizontal)
        }
        .safeAreaInset(edge: .bottom) {
            // Floating action area
            HStack {
                Button("Cancel") { }
                    .buttonStyle(.bordered)
                Spacer()
                Button("Confirm") { }
                    .buttonStyle(.borderedProminent)
            }
            .padding()
            .background(.regularMaterial)
        }
    }
}
```

### Adaptive Layouts

```swift
struct AdaptiveGridView: View {
    @Environment(\.horizontalSizeClass) private var sizeClass

    private var columns: [GridItem] {
        switch sizeClass {
        case .compact:
            return [GridItem(.flexible())]
        case .regular:
            return [
                GridItem(.flexible()),
                GridItem(.flexible()),
                GridItem(.flexible())
            ]
        default:
            return [GridItem(.flexible())]
        }
    }

    var body: some View {
        ScrollView {
            LazyVGrid(columns: columns, spacing: 16) {
                ForEach(items) { item in
                    ItemCard(item: item)
                }
            }
            .padding()
        }
    }
}
```

## Typography Hierarchy

### System Font Styles

```swift
// HIG-compliant typography scale
struct Typography {
    // Titles
    static let largeTitle = Font.largeTitle.weight(.bold)      // 34pt bold
    static let title = Font.title.weight(.semibold)            // 28pt semibold
    static let title2 = Font.title2.weight(.semibold)          // 22pt semibold
    static let title3 = Font.title3.weight(.semibold)          // 20pt semibold

    // Headlines and body
    static let headline = Font.headline                         // 17pt semibold
    static let body = Font.body                                 // 17pt regular
    static let callout = Font.callout                          // 16pt regular

    // Supporting text
    static let subheadline = Font.subheadline                  // 15pt regular
    static let footnote = Font.footnote                        // 13pt regular
    static let caption = Font.caption                          // 12pt regular
    static let caption2 = Font.caption2                        // 11pt regular
}
```

### Custom Font with Dynamic Type

```swift
extension Font {
    static func customBody(_ name: String) -> Font {
        .custom(name, size: 17, relativeTo: .body)
    }

    static func customHeadline(_ name: String) -> Font {
        .custom(name, size: 17, relativeTo: .headline)
            .weight(.semibold)
    }
}

// Usage
Text("Custom styled text")
    .font(.customBody("Avenir Next"))
```

## Color System

### Semantic Colors

```swift
// Use semantic colors for automatic light/dark mode support
extension Color {
    // Labels
    static let primaryLabel = Color.primary
    static let secondaryLabel = Color.secondary
    static let tertiaryLabel = Color(uiColor: .tertiaryLabel)

    // Backgrounds
    static let systemBackground = Color(uiColor: .systemBackground)
    static let secondaryBackground = Color(uiColor: .secondarySystemBackground)
    static let groupedBackground = Color(uiColor: .systemGroupedBackground)

    // Fills
    static let primaryFill = Color(uiColor: .systemFill)
    static let secondaryFill = Color(uiColor: .secondarySystemFill)

    // Separators
    static let separator = Color(uiColor: .separator)
    static let opaqueSeparator = Color(uiColor: .opaqueSeparator)
}
```

### Tint Colors

```swift
// App-wide tint color
struct AppColors {
    static let primary = Color.blue
    static let secondary = Color.purple
    static let success = Color.green
    static let warning = Color.orange
    static let error = Color.red

    // Semantic tints
    static let interactive = Color.accentColor
    static let destructive = Color.red
}

// Apply tint to views
ContentView()
    .tint(AppColors.primary)
```

## Navigation Patterns

### Hierarchical Navigation

```swift
struct MasterDetailView: View {
    @State private var selectedItem: Item?
    @Environment(\.horizontalSizeClass) private var sizeClass

    var body: some View {
        NavigationSplitView {
            // Sidebar
            List(items, selection: $selectedItem) { item in
                NavigationLink(value: item) {
                    ItemRow(item: item)
                }
            }
            .navigationTitle("Items")
            .toolbar {
                ToolbarItem(placement: .primaryAction) {
                    Button("Add", systemImage: "plus") { }
                }
            }
        } detail: {
            // Detail view
            if let item = selectedItem {
                ItemDetailView(item: item)
            } else {
                ContentUnavailableView(
                    "Select an Item",
                    systemImage: "sidebar.leading"
                )
            }
        }
        .navigationSplitViewStyle(.balanced)
    }
}
```

### Tab-Based Navigation

```swift
struct MainTabView: View {
    @State private var selectedTab: Tab = .home

    enum Tab: String, CaseIterable {
        case home, explore, notifications, profile

        var title: String {
            rawValue.capitalized
        }

        var systemImage: String {
            switch self {
            case .home: return "house"
            case .explore: return "magnifyingglass"
            case .notifications: return "bell"
            case .profile: return "person"
            }
        }
    }

    var body: some View {
        TabView(selection: $selectedTab) {
            ForEach(Tab.allCases, id: \.self) { tab in
                NavigationStack {
                    tabContent(for: tab)
                }
                .tabItem {
                    Label(tab.title, systemImage: tab.systemImage)
                }
                .tag(tab)
            }
        }
    }

    @ViewBuilder
    private func tabContent(for tab: Tab) -> some View {
        switch tab {
        case .home:
            HomeView()
        case .explore:
            ExploreView()
        case .notifications:
            NotificationsView()
        case .profile:
            ProfileView()
        }
    }
}
```

## Toolbar Patterns

### Standard Toolbar Items

```swift
struct ContentView: View {
    @State private var isEditing = false

    var body: some View {
        NavigationStack {
            List { /* content */ }
            .navigationTitle("Items")
            .toolbar {
                // Leading items
                ToolbarItem(placement: .topBarLeading) {
                    EditButton()
                }

                // Trailing items
                ToolbarItemGroup(placement: .topBarTrailing) {
                    Button("Filter", systemImage: "line.3.horizontal.decrease.circle") { }
                    Button("Add", systemImage: "plus") { }
                }

                // Bottom bar
                ToolbarItemGroup(placement: .bottomBar) {
                    Button("Archive", systemImage: "archivebox") { }
                    Spacer()
                    Text("\(itemCount) items")
                        .font(.footnote)
                        .foregroundStyle(.secondary)
                    Spacer()
                    Button("Share", systemImage: "square.and.arrow.up") { }
                }
            }
            .toolbarBackground(.visible, for: .bottomBar)
        }
    }
}
```

### Search Integration

```swift
struct SearchableView: View {
    @State private var searchText = ""
    @State private var searchScope: SearchScope = .all
    @State private var isSearching = false

    enum SearchScope: String, CaseIterable {
        case all, titles, content
    }

    var body: some View {
        NavigationStack {
            List(filteredItems) { item in
                ItemRow(item: item)
            }
            .navigationTitle("Library")
            .searchable(
                text: $searchText,
                isPresented: $isSearching,
                placement: .navigationBarDrawer(displayMode: .always)
            )
            .searchScopes($searchScope) {
                ForEach(SearchScope.allCases, id: \.self) { scope in
                    Text(scope.rawValue.capitalized).tag(scope)
                }
            }
        }
    }
}
```

## Feedback Patterns

### Haptic Feedback

```swift
struct HapticFeedback {
    static func impact(_ style: UIImpactFeedbackGenerator.FeedbackStyle = .medium) {
        let generator = UIImpactFeedbackGenerator(style: style)
        generator.impactOccurred()
    }

    static func notification(_ type: UINotificationFeedbackGenerator.FeedbackType) {
        let generator = UINotificationFeedbackGenerator()
        generator.notificationOccurred(type)
    }

    static func selection() {
        let generator = UISelectionFeedbackGenerator()
        generator.selectionChanged()
    }
}

// Usage
Button("Submit") {
    HapticFeedback.notification(.success)
    submit()
}
```

### Visual Feedback

```swift
struct FeedbackButton: View {
    let title: String
    let action: () -> Void

    @State private var showSuccess = false

    var body: some View {
        Button(title) {
            action()
            withAnimation {
                showSuccess = true
            }
            DispatchQueue.main.asyncAfter(deadline: .now() + 1.5) {
                withAnimation {
                    showSuccess = false
                }
            }
        }
        .overlay(alignment: .trailing) {
            if showSuccess {
                Image(systemName: "checkmark.circle.fill")
                    .foregroundStyle(.green)
                    .transition(.scale.combined(with: .opacity))
            }
        }
    }
}
```

## Accessibility

### VoiceOver Support

```swift
struct AccessibleCard: View {
    let item: Item

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            Text(item.title)
                .font(.headline)
            Text(item.subtitle)
                .font(.subheadline)
                .foregroundStyle(.secondary)

            HStack {
                Image(systemName: "star.fill")
                Text("\(item.rating, specifier: "%.1f")")
            }
        }
        .padding()
        .accessibilityElement(children: .combine)
        .accessibilityLabel("\(item.title), \(item.subtitle)")
        .accessibilityValue("Rating: \(item.rating) stars")
        .accessibilityHint("Double tap to view details")
        .accessibilityAddTraits(.isButton)
    }
}
```

### Dynamic Type Support

```swift
struct DynamicTypeView: View {
    @Environment(\.dynamicTypeSize) private var dynamicTypeSize

    var body: some View {
        Group {
            if dynamicTypeSize.isAccessibilitySize {
                // Stack vertically for accessibility sizes
                VStack(alignment: .leading, spacing: 12) {
                    leadingContent
                    trailingContent
                }
            } else {
                // Side-by-side for standard sizes
                HStack {
                    leadingContent
                    Spacer()
                    trailingContent
                }
            }
        }
    }

    var leadingContent: some View {
        Label("Items", systemImage: "folder")
    }

    var trailingContent: some View {
        Text("12")
            .foregroundStyle(.secondary)
    }
}
```

## Error Handling UI

### Error States

```swift
struct ErrorView: View {
    let error: Error
    let retryAction: () async -> Void

    var body: some View {
        ContentUnavailableView {
            Label("Unable to Load", systemImage: "exclamationmark.triangle")
        } description: {
            Text(error.localizedDescription)
        } actions: {
            Button("Try Again") {
                Task {
                    await retryAction()
                }
            }
            .buttonStyle(.borderedProminent)
        }
    }
}
```

### Empty States

```swift
struct EmptyStateView: View {
    let title: String
    let description: String
    let systemImage: String
    let action: (() -> Void)?
    let actionTitle: String?

    var body: some View {
        ContentUnavailableView {
            Label(title, systemImage: systemImage)
        } description: {
            Text(description)
        } actions: {
            if let action, let actionTitle {
                Button(actionTitle, action: action)
                    .buttonStyle(.borderedProminent)
            }
        }
    }
}

// Usage
EmptyStateView(
    title: "No Photos",
    description: "Take your first photo to get started.",
    systemImage: "camera",
    action: { showCamera = true },
    actionTitle: "Take Photo"
)
```


---

## อ้างอิง: ios-navigation

# iOS Navigation Patterns

## NavigationStack (iOS 16+)

### Basic Navigation

```swift
struct BasicNavigationView: View {
    var body: some View {
        NavigationStack {
            List(items) { item in
                NavigationLink(item.title, value: item)
            }
            .navigationTitle("Items")
            .navigationDestination(for: Item.self) { item in
                ItemDetailView(item: item)
            }
        }
    }
}
```

### Programmatic Navigation

```swift
struct ProgrammaticNavigationView: View {
    @State private var path = NavigationPath()

    var body: some View {
        NavigationStack(path: $path) {
            VStack(spacing: 20) {
                Button("Go to Settings") {
                    path.append(Destination.settings)
                }

                Button("Go to Profile") {
                    path.append(Destination.profile)
                }

                Button("Deep Link to Item 123") {
                    path.append(Destination.settings)
                    path.append(Destination.itemDetail(id: 123))
                }
            }
            .navigationTitle("Home")
            .navigationDestination(for: Destination.self) { destination in
                switch destination {
                case .settings:
                    SettingsView()
                case .profile:
                    ProfileView()
                case .itemDetail(let id):
                    ItemDetailView(itemId: id)
                }
            }
        }
    }

    enum Destination: Hashable {
        case settings
        case profile
        case itemDetail(id: Int)
    }
}
```

### Navigation State Persistence

```swift
struct PersistentNavigationView: View {
    @SceneStorage("navigationPath") private var pathData: Data?
    @State private var path = NavigationPath()

    var body: some View {
        NavigationStack(path: $path) {
            ContentView()
                .navigationDestination(for: Item.self) { item in
                    ItemDetailView(item: item)
                }
        }
        .onAppear {
            restorePath()
        }
        .onChange(of: path) { _, newPath in
            savePath(newPath)
        }
    }

    private func savePath(_ path: NavigationPath) {
        guard let representation = path.codable else { return }
        pathData = try? JSONEncoder().encode(representation)
    }

    private func restorePath() {
        guard let data = pathData,
              let representation = try? JSONDecoder().decode(
                NavigationPath.CodableRepresentation.self,
                from: data
              ) else { return }
        path = NavigationPath(representation)
    }
}
```

## NavigationSplitView

### Two-Column Layout

```swift
struct TwoColumnView: View {
    @State private var selectedCategory: Category?
    @State private var columnVisibility: NavigationSplitViewVisibility = .all

    var body: some View {
        NavigationSplitView(columnVisibility: $columnVisibility) {
            // Sidebar
            List(categories, selection: $selectedCategory) { category in
                NavigationLink(value: category) {
                    Label(category.name, systemImage: category.icon)
                }
            }
            .navigationTitle("Categories")
        } detail: {
            // Detail
            if let category = selectedCategory {
                CategoryDetailView(category: category)
            } else {
                ContentUnavailableView(
                    "Select a Category",
                    systemImage: "sidebar.leading"
                )
            }
        }
        .navigationSplitViewStyle(.balanced)
    }
}
```

### Three-Column Layout

```swift
struct ThreeColumnView: View {
    @State private var selectedFolder: Folder?
    @State private var selectedDocument: Document?

    var body: some View {
        NavigationSplitView {
            // Sidebar
            List(folders, selection: $selectedFolder) { folder in
                NavigationLink(value: folder) {
                    Label(folder.name, systemImage: "folder")
                }
            }
            .navigationTitle("Folders")
        } content: {
            // Content column
            if let folder = selectedFolder {
                List(folder.documents, selection: $selectedDocument) { document in
                    NavigationLink(value: document) {
                        DocumentRow(document: document)
                    }
                }
                .navigationTitle(folder.name)
            } else {
                Text("Select a folder")
            }
        } detail: {
            // Detail column
            if let document = selectedDocument {
                DocumentDetailView(document: document)
            } else {
                ContentUnavailableView(
                    "Select a Document",
                    systemImage: "doc"
                )
            }
        }
    }
}
```

## Sheet Navigation

### Modal Sheets

```swift
struct SheetNavigationView: View {
    @State private var showSettings = false
    @State private var showNewItem = false
    @State private var editingItem: Item?

    var body: some View {
        NavigationStack {
            ContentView()
                .toolbar {
                    ToolbarItem(placement: .primaryAction) {
                        Button("Add", systemImage: "plus") {
                            showNewItem = true
                        }
                    }
                    ToolbarItem(placement: .topBarLeading) {
                        Button("Settings", systemImage: "gear") {
                            showSettings = true
                        }
                    }
                }
        }
        // Boolean-based sheet
        .sheet(isPresented: $showSettings) {
            SettingsSheet()
        }
        // Boolean-based fullscreen cover
        .fullScreenCover(isPresented: $showNewItem) {
            NewItemView()
        }
        // Item-based sheet
        .sheet(item: $editingItem) { item in
            EditItemSheet(item: item)
        }
    }
}
```

### Sheet with Navigation

```swift
struct NavigableSheet: View {
    @Environment(\.dismiss) private var dismiss

    var body: some View {
        NavigationStack {
            Form {
                Section("General") {
                    NavigationLink("Account") {
                        AccountSettingsView()
                    }
                    NavigationLink("Notifications") {
                        NotificationSettingsView()
                    }
                }

                Section("Advanced") {
                    NavigationLink("Privacy") {
                        PrivacySettingsView()
                    }
                }
            }
            .navigationTitle("Settings")
            .toolbar {
                ToolbarItem(placement: .confirmationAction) {
                    Button("Done") {
                        dismiss()
                    }
                }
            }
        }
    }
}
```

### Sheet Customization

```swift
struct CustomSheetView: View {
    @State private var showSheet = false

    var body: some View {
        Button("Show Sheet") {
            showSheet = true
        }
        .sheet(isPresented: $showSheet) {
            SheetContent()
                // Available detents
                .presentationDetents([
                    .medium,
                    .large,
                    .height(200),
                    .fraction(0.75)
                ])
                // Selected detent binding
                .presentationDetents([.medium, .large], selection: $selectedDetent)
                // Drag indicator visibility
                .presentationDragIndicator(.visible)
                // Corner radius
                .presentationCornerRadius(24)
                // Background interaction
                .presentationBackgroundInteraction(.enabled(upThrough: .medium))
                // Prevent interactive dismiss
                .interactiveDismissDisabled(hasUnsavedChanges)
        }
    }
}
```

## Tab Navigation

### Basic TabView (iOS 18+)

```swift
struct MainTabView: View {
    @State private var selectedTab = 0

    var body: some View {
        TabView(selection: $selectedTab) {
            Tab("Home", systemImage: "house", value: 0) {
                HomeView()
            }

            Tab("Search", systemImage: "magnifyingglass", value: 1) {
                SearchView()
            }

            Tab("Profile", systemImage: "person", value: 2) {
                ProfileView()
            }
            .badge(unreadCount)
        }
    }
}
```

### Tab with Custom Badge (iOS 18+)

```swift
struct BadgedTabView: View {
    @State private var selectedTab: AppTab = .home
    @State private var cartCount = 3

    enum AppTab: String, CaseIterable {
        case home, search, cart, profile

        var icon: String {
            switch self {
            case .home: return "house"
            case .search: return "magnifyingglass"
            case .cart: return "cart"
            case .profile: return "person"
            }
        }
    }

    var body: some View {
        TabView(selection: $selectedTab) {
            ForEach(AppTab.allCases, id: \.self) { tab in
                Tab(tab.rawValue.capitalized, systemImage: tab.icon, value: tab) {
                    NavigationStack {
                        contentView(for: tab)
                    }
                }
                .badge(tab == .cart ? cartCount : 0)
            }
        }
    }
}
```

## Deep Linking

### URL-Based Navigation

```swift
struct DeepLinkableApp: App {
    @StateObject private var router = NavigationRouter()

    var body: some Scene {
        WindowGroup {
            ContentView()
                .environmentObject(router)
                .onOpenURL { url in
                    router.handle(url: url)
                }
        }
    }
}

class NavigationRouter: ObservableObject {
    @Published var path = NavigationPath()
    @Published var selectedTab: Tab = .home

    func handle(url: URL) {
        guard url.scheme == "myapp" else { return }

        switch url.host {
        case "item":
            if let id = Int(url.lastPathComponent) {
                selectedTab = .home
                path = NavigationPath()
                path.append(Destination.itemDetail(id: id))
            }
        case "settings":
            selectedTab = .profile
            path = NavigationPath()
            path.append(Destination.settings)
        default:
            break
        }
    }
}
```

### Universal Links

```swift
struct UniversalLinkHandler: View {
    @EnvironmentObject private var router: NavigationRouter

    var body: some View {
        ContentView()
            .onContinueUserActivity(NSUserActivityTypeBrowsingWeb) { activity in
                guard let url = activity.webpageURL else { return }
                handleUniversalLink(url)
            }
    }

    private func handleUniversalLink(_ url: URL) {
        // Parse URL path and navigate accordingly
        let pathComponents = url.pathComponents

        if pathComponents.contains("product"),
           let idString = pathComponents.last,
           let id = Int(idString) {
            router.navigate(to: .product(id: id))
        }
    }
}
```

## Navigation Coordinator Pattern

```swift
@MainActor
class NavigationCoordinator: ObservableObject {
    @Published var path = NavigationPath()
    @Published var sheet: Sheet?
    @Published var fullScreenCover: FullScreenCover?

    enum Sheet: Identifiable {
        case settings
        case newItem
        case editItem(Item)

        var id: String {
            switch self {
            case .settings: return "settings"
            case .newItem: return "newItem"
            case .editItem(let item): return "editItem-\(item.id)"
            }
        }
    }

    enum FullScreenCover: Identifiable {
        case onboarding
        case camera

        var id: String {
            switch self {
            case .onboarding: return "onboarding"
            case .camera: return "camera"
            }
        }
    }

    func push(_ destination: Destination) {
        path.append(destination)
    }

    func pop() {
        guard !path.isEmpty else { return }
        path.removeLast()
    }

    func popToRoot() {
        path = NavigationPath()
    }

    func present(_ sheet: Sheet) {
        self.sheet = sheet
    }

    func presentFullScreen(_ cover: FullScreenCover) {
        self.fullScreenCover = cover
    }

    func dismiss() {
        if fullScreenCover != nil {
            fullScreenCover = nil
        } else if sheet != nil {
            sheet = nil
        }
    }
}
```

## Navigation Transitions (iOS 18+)

### Custom Navigation Transitions

```swift
struct CustomTransitionView: View {
    @Namespace private var namespace

    var body: some View {
        NavigationStack {
            List(items) { item in
                NavigationLink(value: item) {
                    ItemRow(item: item)
                        .matchedTransitionSource(id: item.id, in: namespace)
                }
            }
            .navigationDestination(for: Item.self) { item in
                ItemDetailView(item: item)
                    .navigationTransition(.zoom(sourceID: item.id, in: namespace))
            }
        }
    }
}
```

### Hero Transitions

```swift
struct HeroTransitionView: View {
    @Namespace private var animation
    @State private var selectedItem: Item?

    var body: some View {
        ZStack {
            ScrollView {
                LazyVGrid(columns: columns) {
                    ForEach(items) { item in
                        if selectedItem?.id != item.id {
                            ItemCard(item: item)
                                .matchedGeometryEffect(id: item.id, in: animation)
                                .onTapGesture {
                                    withAnimation(.spring(response: 0.3)) {
                                        selectedItem = item
                                    }
                                }
                        }
                    }
                }
            }

            if let item = selectedItem {
                ItemDetailView(item: item)
                    .matchedGeometryEffect(id: item.id, in: animation)
                    .onTapGesture {
                        withAnimation(.spring(response: 0.3)) {
                            selectedItem = nil
                        }
                    }
            }
        }
    }
}
```


---

## อ้างอิง: swiftui-components

# SwiftUI Component Library

## Lists and Collections

### Basic List

```swift
struct ItemListView: View {
    @State private var items: [Item] = []

    var body: some View {
        List {
            ForEach(items) { item in
                ItemRow(item: item)
            }
            .onDelete(perform: deleteItems)
            .onMove(perform: moveItems)
        }
        .listStyle(.insetGrouped)
        .refreshable {
            await loadItems()
        }
    }

    private func deleteItems(at offsets: IndexSet) {
        items.remove(atOffsets: offsets)
    }

    private func moveItems(from source: IndexSet, to destination: Int) {
        items.move(fromOffsets: source, toOffset: destination)
    }
}
```

### Sectioned List

```swift
struct SectionedListView: View {
    let groupedItems: [String: [Item]]

    var body: some View {
        List {
            ForEach(groupedItems.keys.sorted(), id: \.self) { key in
                Section(header: Text(key)) {
                    ForEach(groupedItems[key] ?? []) { item in
                        ItemRow(item: item)
                    }
                }
            }
        }
        .listStyle(.sidebar)
    }
}
```

### Search Integration

```swift
struct SearchableListView: View {
    @State private var searchText = ""
    @State private var items: [Item] = []

    var filteredItems: [Item] {
        if searchText.isEmpty {
            return items
        }
        return items.filter { $0.name.localizedCaseInsensitiveContains(searchText) }
    }

    var body: some View {
        NavigationStack {
            List(filteredItems) { item in
                ItemRow(item: item)
            }
            .searchable(text: $searchText, prompt: "Search items")
            .searchSuggestions {
                ForEach(searchSuggestions, id: \.self) { suggestion in
                    Text(suggestion)
                        .searchCompletion(suggestion)
                }
            }
            .navigationTitle("Items")
        }
    }
}
```

## Forms and Input

### Settings Form

```swift
struct SettingsView: View {
    @AppStorage("notifications") private var notificationsEnabled = true
    @AppStorage("soundEnabled") private var soundEnabled = true
    @State private var selectedTheme = Theme.system
    @State private var username = ""

    var body: some View {
        Form {
            Section("Account") {
                TextField("Username", text: $username)
                    .textContentType(.username)
                    .autocorrectionDisabled()
            }

            Section("Preferences") {
                Toggle("Enable Notifications", isOn: $notificationsEnabled)
                Toggle("Sound Effects", isOn: $soundEnabled)

                Picker("Theme", selection: $selectedTheme) {
                    ForEach(Theme.allCases) { theme in
                        Text(theme.rawValue).tag(theme)
                    }
                }
            }

            Section("About") {
                LabeledContent("Version", value: "1.0.0")

                Link(destination: URL(string: "https://example.com/privacy")!) {
                    Text("Privacy Policy")
                }
            }
        }
        .navigationTitle("Settings")
    }
}
```

### Custom Input Fields

```swift
struct ValidatedTextField: View {
    let title: String
    @Binding var text: String
    let validation: (String) -> Bool

    @State private var isValid = true
    @FocusState private var isFocused: Bool

    var body: some View {
        VStack(alignment: .leading, spacing: 4) {
            Text(title)
                .font(.caption)
                .foregroundStyle(.secondary)

            TextField(title, text: $text)
                .textFieldStyle(.roundedBorder)
                .focused($isFocused)
                .overlay(
                    RoundedRectangle(cornerRadius: 8)
                        .stroke(borderColor, lineWidth: 1)
                )
                .onChange(of: text) { _, newValue in
                    isValid = validation(newValue)
                }

            if !isValid && !text.isEmpty {
                Text("Invalid input")
                    .font(.caption)
                    .foregroundStyle(.red)
            }
        }
    }

    private var borderColor: Color {
        if isFocused {
            return isValid ? .blue : .red
        }
        return .clear
    }
}
```

## Buttons and Actions

### Button Styles

```swift
// Primary filled button
Button("Continue") {
    // action
}
.buttonStyle(.borderedProminent)
.controlSize(.large)

// Secondary button
Button("Cancel") {
    // action
}
.buttonStyle(.bordered)

// Destructive button
Button("Delete", role: .destructive) {
    // action
}
.buttonStyle(.bordered)

// Custom button style
struct ScaleButtonStyle: ButtonStyle {
    func makeBody(configuration: Configuration) -> some View {
        configuration.label
            .scaleEffect(configuration.isPressed ? 0.95 : 1.0)
            .animation(.easeInOut(duration: 0.1), value: configuration.isPressed)
    }
}
```

### Menu and Context Menu

```swift
// Menu button
Menu {
    Button("Edit", systemImage: "pencil") { }
    Button("Duplicate", systemImage: "doc.on.doc") { }
    Divider()
    Button("Delete", systemImage: "trash", role: .destructive) { }
} label: {
    Image(systemName: "ellipsis.circle")
}

// Context menu on any view
Text("Long press me")
    .contextMenu {
        Button("Copy", systemImage: "doc.on.doc") { }
        Button("Share", systemImage: "square.and.arrow.up") { }
    } preview: {
        ItemPreviewView()
    }
```

## Sheets and Modals

### Sheet Presentation

```swift
struct ParentView: View {
    @State private var showSettings = false
    @State private var selectedItem: Item?

    var body: some View {
        VStack {
            Button("Settings") {
                showSettings = true
            }
        }
        .sheet(isPresented: $showSettings) {
            SettingsSheet()
                .presentationDetents([.medium, .large])
                .presentationDragIndicator(.visible)
        }
        .sheet(item: $selectedItem) { item in
            ItemDetailSheet(item: item)
                .presentationDetents([.height(300), .large])
                .presentationCornerRadius(24)
        }
    }
}

struct SettingsSheet: View {
    @Environment(\.dismiss) private var dismiss

    var body: some View {
        NavigationStack {
            SettingsContent()
                .toolbar {
                    ToolbarItem(placement: .confirmationAction) {
                        Button("Done") {
                            dismiss()
                        }
                    }
                }
        }
    }
}
```

### Confirmation Dialog

```swift
struct DeleteConfirmationView: View {
    @State private var showConfirmation = false

    var body: some View {
        Button("Delete Account", role: .destructive) {
            showConfirmation = true
        }
        .confirmationDialog(
            "Delete Account",
            isPresented: $showConfirmation,
            titleVisibility: .visible
        ) {
            Button("Delete", role: .destructive) {
                deleteAccount()
            }
            Button("Cancel", role: .cancel) { }
        } message: {
            Text("This action cannot be undone.")
        }
    }
}
```

## Loading and Progress

### Progress Indicators

```swift
// Indeterminate spinner
ProgressView()
    .progressViewStyle(.circular)

// Determinate progress
ProgressView(value: downloadProgress, total: 1.0) {
    Text("Downloading...")
} currentValueLabel: {
    Text("\(Int(downloadProgress * 100))%")
}

// Custom loading view
struct LoadingOverlay: View {
    let message: String

    var body: some View {
        ZStack {
            Color.black.opacity(0.4)
                .ignoresSafeArea()

            VStack(spacing: 16) {
                ProgressView()
                    .scaleEffect(1.5)
                    .tint(.white)

                Text(message)
                    .font(.subheadline)
                    .foregroundStyle(.white)
            }
            .padding(24)
            .background(.ultraThinMaterial, in: RoundedRectangle(cornerRadius: 16))
        }
    }
}
```

### Skeleton Loading

```swift
struct SkeletonRow: View {
    @State private var isAnimating = false

    var body: some View {
        HStack(spacing: 12) {
            Circle()
                .fill(.gray.opacity(0.3))
                .frame(width: 44, height: 44)

            VStack(alignment: .leading, spacing: 8) {
                RoundedRectangle(cornerRadius: 4)
                    .fill(.gray.opacity(0.3))
                    .frame(height: 14)
                    .frame(maxWidth: 200)

                RoundedRectangle(cornerRadius: 4)
                    .fill(.gray.opacity(0.2))
                    .frame(height: 12)
                    .frame(maxWidth: 150)
            }
        }
        .opacity(isAnimating ? 0.5 : 1.0)
        .animation(.easeInOut(duration: 0.8).repeatForever(), value: isAnimating)
        .onAppear { isAnimating = true }
    }
}
```

## Async Content Loading

### AsyncImage

```swift
AsyncImage(url: imageURL) { phase in
    switch phase {
    case .empty:
        ProgressView()
    case .success(let image):
        image
            .resizable()
            .aspectRatio(contentMode: .fill)
    case .failure:
        Image(systemName: "photo")
            .foregroundStyle(.secondary)
    @unknown default:
        EmptyView()
    }
}
.frame(width: 100, height: 100)
.clipShape(RoundedRectangle(cornerRadius: 8))
```

### Task-Based Loading

```swift
struct AsyncContentView: View {
    @State private var items: [Item] = []
    @State private var isLoading = true
    @State private var error: Error?

    var body: some View {
        Group {
            if isLoading {
                ProgressView("Loading...")
            } else if let error {
                ContentUnavailableView(
                    "Failed to Load",
                    systemImage: "exclamationmark.triangle",
                    description: Text(error.localizedDescription)
                )
            } else if items.isEmpty {
                ContentUnavailableView(
                    "No Items",
                    systemImage: "tray",
                    description: Text("Add your first item to get started.")
                )
            } else {
                List(items) { item in
                    ItemRow(item: item)
                }
            }
        }
        .task {
            await loadItems()
        }
    }

    private func loadItems() async {
        do {
            items = try await api.fetchItems()
            isLoading = false
        } catch {
            self.error = error
            isLoading = false
        }
    }
}
```

## Animations

### Implicit Animations

```swift
struct AnimatedCard: View {
    @State private var isExpanded = false

    var body: some View {
        VStack {
            Text("Tap to expand")

            if isExpanded {
                Text("Additional content here")
                    .transition(.move(edge: .top).combined(with: .opacity))
            }
        }
        .padding()
        .frame(maxWidth: .infinity)
        .background(.blue.opacity(0.1), in: RoundedRectangle(cornerRadius: 12))
        .onTapGesture {
            withAnimation(.spring(response: 0.3, dampingFraction: 0.7)) {
                isExpanded.toggle()
            }
        }
    }
}
```

### Custom Transitions

```swift
extension AnyTransition {
    static var slideAndFade: AnyTransition {
        .asymmetric(
            insertion: .move(edge: .trailing).combined(with: .opacity),
            removal: .move(edge: .leading).combined(with: .opacity)
        )
    }

    static var scaleAndFade: AnyTransition {
        .scale(scale: 0.8).combined(with: .opacity)
    }
}
```

### Phase Animator (iOS 17+)

```swift
struct PulsingButton: View {
    var body: some View {
        Button("Tap Me") { }
            .buttonStyle(.borderedProminent)
            .phaseAnimator([false, true]) { content, phase in
                content
                    .scaleEffect(phase ? 1.05 : 1.0)
            } animation: { _ in
                .easeInOut(duration: 0.5)
            }
    }
}
```

## Gestures

### Drag Gesture

```swift
struct DraggableCard: View {
    @State private var offset = CGSize.zero
    @State private var isDragging = false

    var body: some View {
        RoundedRectangle(cornerRadius: 16)
            .fill(.blue)
            .frame(width: 200, height: 150)
            .offset(offset)
            .scaleEffect(isDragging ? 1.05 : 1.0)
            .gesture(
                DragGesture()
                    .onChanged { value in
                        offset = value.translation
                        isDragging = true
                    }
                    .onEnded { _ in
                        withAnimation(.spring()) {
                            offset = .zero
                            isDragging = false
                        }
                    }
            )
    }
}
```

### Simultaneous Gestures

```swift
struct ZoomableImage: View {
    @State private var scale: CGFloat = 1.0
    @State private var lastScale: CGFloat = 1.0

    var body: some View {
        Image("photo")
            .resizable()
            .aspectRatio(contentMode: .fit)
            .scaleEffect(scale)
            .gesture(
                MagnificationGesture()
                    .onChanged { value in
                        scale = lastScale * value
                    }
                    .onEnded { _ in
                        lastScale = scale
                    }
            )
            .gesture(
                TapGesture(count: 2)
                    .onEnded {
                        withAnimation {
                            scale = 1.0
                            lastScale = 1.0
                        }
                    }
            )
    }
}
```
