
# Android Mobile Design

Master Material Design 3 (Material You) and Jetpack Compose to build modern, adaptive Android applications that integrate seamlessly with the Android ecosystem.

## When to Use This Skill

- Designing Android app interfaces following Material Design 3
- Building Jetpack Compose UI and layouts
- Implementing Android navigation patterns (Navigation Compose)
- Creating adaptive layouts for phones, tablets, and foldables
- Using Material 3 theming with dynamic colors
- Building accessible Android interfaces
- Implementing Android-specific gestures and interactions
- Designing for different screen configurations

## Detailed section: Core Concepts

The detailed patterns follow below in this file.

## Quick Start Component

```kotlin
@Composable
fun ItemListCard(
    item: Item,
    onItemClick: () -> Unit,
    modifier: Modifier = Modifier
) {
    Card(
        onClick = onItemClick,
        modifier = modifier.fillMaxWidth(),
        shape = RoundedCornerShape(12.dp)
    ) {
        Row(
            modifier = Modifier
                .padding(16.dp)
                .fillMaxWidth(),
            verticalAlignment = Alignment.CenterVertically
        ) {
            Box(
                modifier = Modifier
                    .size(48.dp)
                    .clip(CircleShape)
                    .background(MaterialTheme.colorScheme.primaryContainer),
                contentAlignment = Alignment.Center
            ) {
                Icon(
                    imageVector = Icons.Default.Star,
                    contentDescription = null,
                    tint = MaterialTheme.colorScheme.onPrimaryContainer
                )
            }

            Spacer(modifier = Modifier.width(16.dp))

            Column(modifier = Modifier.weight(1f)) {
                Text(
                    text = item.title,
                    style = MaterialTheme.typography.titleMedium
                )
                Text(
                    text = item.subtitle,
                    style = MaterialTheme.typography.bodyMedium,
                    color = MaterialTheme.colorScheme.onSurfaceVariant
                )
            }

            Icon(
                imageVector = Icons.Default.ChevronRight,
                contentDescription = null,
                tint = MaterialTheme.colorScheme.onSurfaceVariant
            )
        }
    }
}
```

## Best Practices

1. **Use Material Theme**: Access colors via `MaterialTheme.colorScheme` for automatic dark mode support
2. **Support Dynamic Color**: Enable dynamic color on Android 12+ for personalization
3. **Adaptive Layouts**: Use `WindowSizeClass` for responsive designs
4. **Content Descriptions**: Add `contentDescription` to all interactive elements
5. **Touch Targets**: Minimum 48dp touch targets for accessibility
6. **State Hoisting**: Hoist state to make components reusable and testable
7. **Remember Properly**: Use `remember` and `rememberSaveable` appropriately
8. **Preview Annotations**: Add `@Preview` with different configurations

## Common Issues

- **Recomposition Issues**: Avoid passing unstable lambdas; use `remember`
- **State Loss**: Use `rememberSaveable` for configuration changes
- **Performance**: Use `LazyColumn` instead of `Column` for long lists
- **Theme Leaks**: Ensure `MaterialTheme` wraps all composables
- **Navigation Crashes**: Handle back press and deep links properly
- **Memory Leaks**: Cancel coroutines in `DisposableEffect`


---

## อ้างอิง: android-navigation

# Android Navigation Patterns

## Navigation Compose Basics

### Setup and Dependencies

```kotlin
// build.gradle.kts
dependencies {
    implementation("androidx.navigation:navigation-compose:2.7.7")
    // For type-safe navigation (recommended)
    implementation("org.jetbrains.kotlinx:kotlinx-serialization-json:1.6.3")
}
```

### Basic Navigation

```kotlin
@Serializable
object Home

@Serializable
data class Detail(val itemId: String)

@Serializable
object Settings

@Composable
fun AppNavigation() {
    val navController = rememberNavController()

    NavHost(
        navController = navController,
        startDestination = Home
    ) {
        composable<Home> {
            HomeScreen(
                onItemClick = { itemId ->
                    navController.navigate(Detail(itemId))
                },
                onSettingsClick = {
                    navController.navigate(Settings)
                }
            )
        }

        composable<Detail> { backStackEntry ->
            val detail: Detail = backStackEntry.toRoute()
            DetailScreen(
                itemId = detail.itemId,
                onBack = { navController.popBackStack() }
            )
        }

        composable<Settings> {
            SettingsScreen(
                onBack = { navController.popBackStack() }
            )
        }
    }
}
```

### Navigation with Arguments

```kotlin
// Type-safe routes with arguments
@Serializable
data class ProductDetail(
    val productId: String,
    val category: String,
    val fromSearch: Boolean = false
)

@Serializable
data class UserProfile(
    val userId: Long
)

@Composable
fun NavigationWithArgs() {
    val navController = rememberNavController()

    NavHost(navController = navController, startDestination = Home) {
        composable<Home> {
            HomeScreen(
                onProductClick = { productId, category ->
                    navController.navigate(
                        ProductDetail(
                            productId = productId,
                            category = category,
                            fromSearch = false
                        )
                    )
                }
            )
        }

        composable<ProductDetail> { backStackEntry ->
            val args: ProductDetail = backStackEntry.toRoute()
            ProductDetailScreen(
                productId = args.productId,
                category = args.category,
                showBackToSearch = args.fromSearch
            )
        }

        composable<UserProfile> { backStackEntry ->
            val args: UserProfile = backStackEntry.toRoute()
            UserProfileScreen(userId = args.userId)
        }
    }
}
```

## Bottom Navigation

### Standard Implementation

```kotlin
enum class BottomNavDestination(
    val route: Any,
    val icon: ImageVector,
    val label: String
) {
    HOME(Home, Icons.Default.Home, "Home"),
    SEARCH(Search, Icons.Default.Search, "Search"),
    FAVORITES(Favorites, Icons.Default.Favorite, "Favorites"),
    PROFILE(Profile, Icons.Default.Person, "Profile")
}

@Composable
fun MainScreenWithBottomNav() {
    val navController = rememberNavController()
    val navBackStackEntry by navController.currentBackStackEntryAsState()
    val currentDestination = navBackStackEntry?.destination

    Scaffold(
        bottomBar = {
            NavigationBar {
                BottomNavDestination.entries.forEach { destination ->
                    NavigationBarItem(
                        icon = {
                            Icon(destination.icon, contentDescription = destination.label)
                        },
                        label = { Text(destination.label) },
                        selected = currentDestination?.hasRoute(destination.route::class) == true,
                        onClick = {
                            navController.navigate(destination.route) {
                                // Pop up to start destination to avoid building up stack
                                popUpTo(navController.graph.findStartDestination().id) {
                                    saveState = true
                                }
                                // Avoid multiple copies of same destination
                                launchSingleTop = true
                                // Restore state when reselecting
                                restoreState = true
                            }
                        }
                    )
                }
            }
        }
    ) { innerPadding ->
        NavHost(
            navController = navController,
            startDestination = Home,
            modifier = Modifier.padding(innerPadding)
        ) {
            composable<Home> { HomeScreen() }
            composable<Search> { SearchScreen() }
            composable<Favorites> { FavoritesScreen() }
            composable<Profile> { ProfileScreen() }
        }
    }
}
```

### Bottom Nav with Badges

```kotlin
@Composable
fun BottomNavWithBadges(
    cartCount: Int,
    notificationCount: Int
) {
    NavigationBar {
        NavigationBarItem(
            icon = { Icon(Icons.Default.Home, null) },
            label = { Text("Home") },
            selected = true,
            onClick = { }
        )

        NavigationBarItem(
            icon = {
                BadgedBox(
                    badge = {
                        if (cartCount > 0) {
                            Badge { Text("$cartCount") }
                        }
                    }
                ) {
                    Icon(Icons.Default.ShoppingCart, null)
                }
            },
            label = { Text("Cart") },
            selected = false,
            onClick = { }
        )

        NavigationBarItem(
            icon = {
                BadgedBox(
                    badge = {
                        if (notificationCount > 0) {
                            Badge {
                                Text(
                                    if (notificationCount > 99) "99+"
                                    else "$notificationCount"
                                )
                            }
                        }
                    }
                ) {
                    Icon(Icons.Default.Notifications, null)
                }
            },
            label = { Text("Alerts") },
            selected = false,
            onClick = { }
        )
    }
}
```

## Navigation Drawer

### Modal Navigation Drawer

```kotlin
@Composable
fun ModalDrawerNavigation() {
    val drawerState = rememberDrawerState(initialValue = DrawerValue.Closed)
    val scope = rememberCoroutineScope()
    var selectedItem by remember { mutableStateOf(0) }

    val items = listOf(
        DrawerItem(Icons.Default.Home, "Home"),
        DrawerItem(Icons.Default.Settings, "Settings"),
        DrawerItem(Icons.Default.Info, "About"),
        DrawerItem(Icons.Default.Help, "Help")
    )

    ModalNavigationDrawer(
        drawerState = drawerState,
        drawerContent = {
            ModalDrawerSheet {
                // Header
                Box(
                    modifier = Modifier
                        .fillMaxWidth()
                        .height(180.dp)
                        .background(MaterialTheme.colorScheme.primaryContainer),
                    contentAlignment = Alignment.BottomStart
                ) {
                    Column(modifier = Modifier.padding(16.dp)) {
                        AsyncImage(
                            model = "avatar_url",
                            contentDescription = "Profile",
                            modifier = Modifier
                                .size(64.dp)
                                .clip(CircleShape)
                        )
                        Spacer(Modifier.height(8.dp))
                        Text(
                            "John Doe",
                            style = MaterialTheme.typography.titleMedium
                        )
                        Text(
                            "john@example.com",
                            style = MaterialTheme.typography.bodySmall
                        )
                    }
                }

                Spacer(Modifier.height(12.dp))

                // Navigation items
                items.forEachIndexed { index, item ->
                    NavigationDrawerItem(
                        icon = { Icon(item.icon, contentDescription = null) },
                        label = { Text(item.label) },
                        selected = index == selectedItem,
                        onClick = {
                            selectedItem = index
                            scope.launch { drawerState.close() }
                        },
                        modifier = Modifier.padding(NavigationDrawerItemDefaults.ItemPadding)
                    )
                }

                Spacer(Modifier.weight(1f))

                // Footer
                HorizontalDivider()
                NavigationDrawerItem(
                    icon = { Icon(Icons.Default.Logout, null) },
                    label = { Text("Sign Out") },
                    selected = false,
                    onClick = { },
                    modifier = Modifier.padding(NavigationDrawerItemDefaults.ItemPadding)
                )
                Spacer(Modifier.height(12.dp))
            }
        }
    ) {
        Scaffold(
            topBar = {
                TopAppBar(
                    title = { Text(items[selectedItem].label) },
                    navigationIcon = {
                        IconButton(onClick = { scope.launch { drawerState.open() } }) {
                            Icon(Icons.Default.Menu, "Open drawer")
                        }
                    }
                )
            }
        ) { padding ->
            Content(modifier = Modifier.padding(padding))
        }
    }
}

data class DrawerItem(val icon: ImageVector, val label: String)
```

### Permanent Navigation Drawer (Tablets)

```kotlin
@Composable
fun PermanentDrawerLayout() {
    PermanentNavigationDrawer(
        drawerContent = {
            PermanentDrawerSheet(
                modifier = Modifier.width(240.dp)
            ) {
                Spacer(Modifier.height(12.dp))
                Text(
                    "App Name",
                    modifier = Modifier.padding(16.dp),
                    style = MaterialTheme.typography.titleLarge
                )
                HorizontalDivider()

                drawerItems.forEach { item ->
                    NavigationDrawerItem(
                        icon = { Icon(item.icon, null) },
                        label = { Text(item.label) },
                        selected = item == selectedItem,
                        onClick = { selectedItem = item },
                        modifier = Modifier.padding(horizontal = 12.dp)
                    )
                }
            }
        }
    ) {
        // Main content takes remaining space
        MainContent()
    }
}
```

## Navigation Rail

```kotlin
@Composable
fun NavigationRailLayout() {
    var selectedItem by remember { mutableStateOf(0) }

    Row(modifier = Modifier.fillMaxSize()) {
        NavigationRail(
            header = {
                FloatingActionButton(
                    onClick = { },
                    elevation = FloatingActionButtonDefaults.bottomAppBarFabElevation()
                ) {
                    Icon(Icons.Default.Add, "Create")
                }
            }
        ) {
            Spacer(Modifier.weight(1f))

            railItems.forEachIndexed { index, item ->
                NavigationRailItem(
                    icon = { Icon(item.icon, null) },
                    label = { Text(item.label) },
                    selected = selectedItem == index,
                    onClick = { selectedItem = index }
                )
            }

            Spacer(Modifier.weight(1f))
        }

        // Main content
        Box(
            modifier = Modifier
                .weight(1f)
                .fillMaxHeight()
        ) {
            when (selectedItem) {
                0 -> HomeContent()
                1 -> SearchContent()
                2 -> ProfileContent()
            }
        }
    }
}
```

## Deep Linking

### Basic Deep Link Setup

```kotlin
// In AndroidManifest.xml
// <intent-filter>
//     <action android:name="android.intent.action.VIEW" />
//     <category android:name="android.intent.category.DEFAULT" />
//     <category android:name="android.intent.category.BROWSABLE" />
//     <data android:scheme="myapp" />
//     <data android:scheme="https" android:host="myapp.com" />
// </intent-filter>

@Composable
fun DeepLinkNavigation() {
    val navController = rememberNavController()

    NavHost(
        navController = navController,
        startDestination = Home
    ) {
        composable<Home> {
            HomeScreen()
        }

        composable<ProductDetail>(
            deepLinks = listOf(
                navDeepLink<ProductDetail>(
                    basePath = "https://myapp.com/product"
                ),
                navDeepLink<ProductDetail>(
                    basePath = "myapp://product"
                )
            )
        ) { backStackEntry ->
            val args: ProductDetail = backStackEntry.toRoute()
            ProductDetailScreen(productId = args.productId)
        }

        composable<UserProfile>(
            deepLinks = listOf(
                navDeepLink<UserProfile>(
                    basePath = "https://myapp.com/user"
                )
            )
        ) { backStackEntry ->
            val args: UserProfile = backStackEntry.toRoute()
            UserProfileScreen(userId = args.userId)
        }
    }
}
```

### Handling Intent in Activity

```kotlin
class MainActivity : ComponentActivity() {
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)

        setContent {
            AppTheme {
                val navController = rememberNavController()

                // Handle deep link from intent
                LaunchedEffect(Unit) {
                    intent?.data?.let { uri ->
                        navController.handleDeepLink(intent)
                    }
                }

                AppNavigation(navController = navController)
            }
        }
    }

    override fun onNewIntent(intent: Intent) {
        super.onNewIntent(intent)
        // Handle new intents when activity is already running
        setIntent(intent)
    }
}
```

## Nested Navigation

```kotlin
@Composable
fun NestedNavigation() {
    val navController = rememberNavController()

    NavHost(navController = navController, startDestination = MainGraph) {
        // Main graph with bottom navigation
        navigation<MainGraph>(startDestination = Home) {
            composable<Home> {
                HomeScreen(
                    onItemClick = { navController.navigate(Detail(it)) }
                )
            }
            composable<Search> { SearchScreen() }
            composable<Profile> {
                ProfileScreen(
                    onSettingsClick = { navController.navigate(SettingsGraph) }
                )
            }
        }

        // Nested detail graph
        composable<Detail> { backStackEntry ->
            val args: Detail = backStackEntry.toRoute()
            DetailScreen(itemId = args.itemId)
        }

        // Separate settings graph (full screen, no bottom nav)
        navigation<SettingsGraph>(startDestination = SettingsMain) {
            composable<SettingsMain> {
                SettingsScreen(
                    onAccountClick = { navController.navigate(AccountSettings) },
                    onNotificationsClick = { navController.navigate(NotificationSettings) }
                )
            }
            composable<AccountSettings> { AccountSettingsScreen() }
            composable<NotificationSettings> { NotificationSettingsScreen() }
        }
    }
}

@Serializable object MainGraph
@Serializable object SettingsGraph
@Serializable object SettingsMain
@Serializable object AccountSettings
@Serializable object NotificationSettings
```

## Navigation State Management

### ViewModel Integration

```kotlin
@HiltViewModel
class NavigationViewModel @Inject constructor(
    private val savedStateHandle: SavedStateHandle
) : ViewModel() {

    private val _navigationEvents = MutableSharedFlow<NavigationEvent>()
    val navigationEvents = _navigationEvents.asSharedFlow()

    fun navigateToDetail(itemId: String) {
        viewModelScope.launch {
            _navigationEvents.emit(NavigationEvent.NavigateToDetail(itemId))
        }
    }

    fun navigateBack() {
        viewModelScope.launch {
            _navigationEvents.emit(NavigationEvent.NavigateBack)
        }
    }
}

sealed class NavigationEvent {
    data class NavigateToDetail(val itemId: String) : NavigationEvent()
    object NavigateBack : NavigationEvent()
}

@Composable
fun NavigationHandler(
    navController: NavHostController,
    viewModel: NavigationViewModel = hiltViewModel()
) {
    LaunchedEffect(Unit) {
        viewModel.navigationEvents.collect { event ->
            when (event) {
                is NavigationEvent.NavigateToDetail -> {
                    navController.navigate(Detail(event.itemId))
                }
                NavigationEvent.NavigateBack -> {
                    navController.popBackStack()
                }
            }
        }
    }
}
```

### Back Handler

```kotlin
@Composable
fun ScreenWithBackHandler(
    onBack: () -> Unit
) {
    var showExitDialog by remember { mutableStateOf(false) }

    // Intercept back press
    BackHandler {
        showExitDialog = true
    }

    if (showExitDialog) {
        AlertDialog(
            onDismissRequest = { showExitDialog = false },
            title = { Text("Exit App?") },
            text = { Text("Are you sure you want to exit?") },
            confirmButton = {
                TextButton(onClick = onBack) {
                    Text("Exit")
                }
            },
            dismissButton = {
                TextButton(onClick = { showExitDialog = false }) {
                    Text("Cancel")
                }
            }
        )
    }

    // Screen content
    Content()
}
```

## Navigation Animations

```kotlin
@Composable
fun AnimatedNavigation() {
    val navController = rememberNavController()

    NavHost(
        navController = navController,
        startDestination = Home,
        enterTransition = {
            slideIntoContainer(
                towards = AnimatedContentTransitionScope.SlideDirection.Left,
                animationSpec = tween(300)
            )
        },
        exitTransition = {
            slideOutOfContainer(
                towards = AnimatedContentTransitionScope.SlideDirection.Left,
                animationSpec = tween(300)
            )
        },
        popEnterTransition = {
            slideIntoContainer(
                towards = AnimatedContentTransitionScope.SlideDirection.Right,
                animationSpec = tween(300)
            )
        },
        popExitTransition = {
            slideOutOfContainer(
                towards = AnimatedContentTransitionScope.SlideDirection.Right,
                animationSpec = tween(300)
            )
        }
    ) {
        composable<Home> {
            HomeScreen()
        }

        composable<Detail>(
            // Custom transition for specific route
            enterTransition = {
                fadeIn(animationSpec = tween(500)) +
                    scaleIn(initialScale = 0.9f, animationSpec = tween(500))
            },
            exitTransition = {
                fadeOut(animationSpec = tween(500))
            }
        ) {
            DetailScreen()
        }
    }
}
```


---

## อ้างอิง: compose-components

# Jetpack Compose Component Library

## Lists and Collections

### Basic LazyColumn

```kotlin
@Composable
fun ItemList(
    items: List<Item>,
    onItemClick: (Item) -> Unit,
    modifier: Modifier = Modifier
) {
    LazyColumn(
        modifier = modifier.fillMaxSize(),
        contentPadding = PaddingValues(16.dp),
        verticalArrangement = Arrangement.spacedBy(8.dp)
    ) {
        items(
            items = items,
            key = { it.id }
        ) { item ->
            ItemRow(
                item = item,
                onClick = { onItemClick(item) }
            )
        }
    }
}
```

### Pull to Refresh

```kotlin
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun RefreshableList(
    items: List<Item>,
    isRefreshing: Boolean,
    onRefresh: () -> Unit
) {
    val pullToRefreshState = rememberPullToRefreshState()

    PullToRefreshBox(
        state = pullToRefreshState,
        isRefreshing = isRefreshing,
        onRefresh = onRefresh
    ) {
        LazyColumn(
            modifier = Modifier.fillMaxSize()
        ) {
            items(items) { item ->
                ItemRow(item = item)
            }
        }
    }
}
```

### Swipe to Dismiss

```kotlin
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun SwipeableItem(
    item: Item,
    onDelete: () -> Unit
) {
    val dismissState = rememberSwipeToDismissBoxState(
        confirmValueChange = { value ->
            if (value == SwipeToDismissBoxValue.EndToStart) {
                onDelete()
                true
            } else {
                false
            }
        }
    )

    SwipeToDismissBox(
        state = dismissState,
        backgroundContent = {
            Box(
                modifier = Modifier
                    .fillMaxSize()
                    .background(MaterialTheme.colorScheme.errorContainer)
                    .padding(horizontal = 20.dp),
                contentAlignment = Alignment.CenterEnd
            ) {
                Icon(
                    Icons.Default.Delete,
                    contentDescription = "Delete",
                    tint = MaterialTheme.colorScheme.onErrorContainer
                )
            }
        }
    ) {
        ItemRow(item = item)
    }
}
```

### Sticky Headers

```kotlin
@OptIn(ExperimentalFoundationApi::class)
@Composable
fun GroupedList(
    groups: Map<String, List<Item>>
) {
    LazyColumn {
        groups.forEach { (header, items) ->
            stickyHeader {
                Surface(
                    modifier = Modifier.fillMaxWidth(),
                    color = MaterialTheme.colorScheme.surfaceVariant
                ) {
                    Text(
                        text = header,
                        modifier = Modifier.padding(16.dp),
                        style = MaterialTheme.typography.titleSmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant
                    )
                }
            }
            items(items, key = { it.id }) { item ->
                ItemRow(item = item)
            }
        }
    }
}
```

## Forms and Input

### Text Fields

```kotlin
@Composable
fun LoginForm(
    onLogin: (email: String, password: String) -> Unit
) {
    var email by rememberSaveable { mutableStateOf("") }
    var password by rememberSaveable { mutableStateOf("") }
    var passwordVisible by rememberSaveable { mutableStateOf(false) }
    var emailError by remember { mutableStateOf<String?>(null) }

    Column(
        modifier = Modifier.padding(16.dp),
        verticalArrangement = Arrangement.spacedBy(16.dp)
    ) {
        OutlinedTextField(
            value = email,
            onValueChange = {
                email = it
                emailError = if (it.isValidEmail()) null else "Invalid email"
            },
            label = { Text("Email") },
            placeholder = { Text("name@example.com") },
            leadingIcon = { Icon(Icons.Default.Email, null) },
            isError = emailError != null,
            supportingText = emailError?.let { { Text(it) } },
            keyboardOptions = KeyboardOptions(
                keyboardType = KeyboardType.Email,
                imeAction = ImeAction.Next
            ),
            singleLine = true,
            modifier = Modifier.fillMaxWidth()
        )

        OutlinedTextField(
            value = password,
            onValueChange = { password = it },
            label = { Text("Password") },
            leadingIcon = { Icon(Icons.Default.Lock, null) },
            trailingIcon = {
                IconButton(onClick = { passwordVisible = !passwordVisible }) {
                    Icon(
                        if (passwordVisible) Icons.Default.VisibilityOff
                        else Icons.Default.Visibility,
                        contentDescription = "Toggle password visibility"
                    )
                }
            },
            visualTransformation = if (passwordVisible)
                VisualTransformation.None
            else
                PasswordVisualTransformation(),
            keyboardOptions = KeyboardOptions(
                keyboardType = KeyboardType.Password,
                imeAction = ImeAction.Done
            ),
            singleLine = true,
            modifier = Modifier.fillMaxWidth()
        )

        Button(
            onClick = { onLogin(email, password) },
            modifier = Modifier.fillMaxWidth(),
            enabled = email.isNotEmpty() && password.isNotEmpty() && emailError == null
        ) {
            Text("Sign In")
        }
    }
}
```

### Search Bar

```kotlin
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun SearchableScreen(
    items: List<Item>,
    onItemClick: (Item) -> Unit
) {
    var query by rememberSaveable { mutableStateOf("") }
    var expanded by rememberSaveable { mutableStateOf(false) }

    val filteredItems = remember(query, items) {
        if (query.isEmpty()) items
        else items.filter { it.name.contains(query, ignoreCase = true) }
    }

    SearchBar(
        query = query,
        onQueryChange = { query = it },
        onSearch = { expanded = false },
        active = expanded,
        onActiveChange = { expanded = it },
        placeholder = { Text("Search items") },
        leadingIcon = { Icon(Icons.Default.Search, null) },
        trailingIcon = {
            if (query.isNotEmpty()) {
                IconButton(onClick = { query = "" }) {
                    Icon(Icons.Default.Clear, "Clear search")
                }
            }
        },
        modifier = Modifier
            .fillMaxWidth()
            .padding(horizontal = if (expanded) 0.dp else 16.dp)
    ) {
        LazyColumn(
            modifier = Modifier.fillMaxWidth(),
            contentPadding = PaddingValues(16.dp)
        ) {
            items(filteredItems) { item ->
                ListItem(
                    headlineContent = { Text(item.name) },
                    supportingContent = { Text(item.description) },
                    modifier = Modifier.clickable {
                        onItemClick(item)
                        expanded = false
                    }
                )
            }
        }
    }
}
```

### Selection Controls

```kotlin
@Composable
fun SettingsScreen() {
    var notificationsEnabled by rememberSaveable { mutableStateOf(true) }
    var selectedOption by rememberSaveable { mutableStateOf(0) }
    var expandedDropdown by remember { mutableStateOf(false) }
    var selectedLanguage by rememberSaveable { mutableStateOf("English") }
    val languages = listOf("English", "Spanish", "French", "German")

    Column {
        // Switch
        ListItem(
            headlineContent = { Text("Enable Notifications") },
            supportingContent = { Text("Receive push notifications") },
            trailingContent = {
                Switch(
                    checked = notificationsEnabled,
                    onCheckedChange = { notificationsEnabled = it }
                )
            }
        )

        HorizontalDivider()

        // Radio buttons
        Column {
            Text(
                "Theme",
                modifier = Modifier.padding(16.dp),
                style = MaterialTheme.typography.titleSmall
            )
            listOf("System", "Light", "Dark").forEachIndexed { index, option ->
                Row(
                    modifier = Modifier
                        .fillMaxWidth()
                        .selectable(
                            selected = selectedOption == index,
                            onClick = { selectedOption = index },
                            role = Role.RadioButton
                        )
                        .padding(horizontal = 16.dp, vertical = 12.dp),
                    verticalAlignment = Alignment.CenterVertically
                ) {
                    RadioButton(
                        selected = selectedOption == index,
                        onClick = null
                    )
                    Spacer(Modifier.width(16.dp))
                    Text(option)
                }
            }
        }

        HorizontalDivider()

        // Dropdown
        ExposedDropdownMenuBox(
            expanded = expandedDropdown,
            onExpandedChange = { expandedDropdown = it },
            modifier = Modifier.padding(16.dp)
        ) {
            OutlinedTextField(
                value = selectedLanguage,
                onValueChange = {},
                readOnly = true,
                label = { Text("Language") },
                trailingIcon = { ExposedDropdownMenuDefaults.TrailingIcon(expanded = expandedDropdown) },
                modifier = Modifier
                    .fillMaxWidth()
                    .menuAnchor()
            )
            ExposedDropdownMenu(
                expanded = expandedDropdown,
                onDismissRequest = { expandedDropdown = false }
            ) {
                languages.forEach { language ->
                    DropdownMenuItem(
                        text = { Text(language) },
                        onClick = {
                            selectedLanguage = language
                            expandedDropdown = false
                        },
                        contentPadding = ExposedDropdownMenuDefaults.ItemContentPadding
                    )
                }
            }
        }
    }
}
```

## Dialogs and Bottom Sheets

### Alert Dialog

```kotlin
@Composable
fun DeleteConfirmationDialog(
    itemName: String,
    onConfirm: () -> Unit,
    onDismiss: () -> Unit
) {
    AlertDialog(
        onDismissRequest = onDismiss,
        icon = {
            Icon(
                Icons.Default.Warning,
                contentDescription = null,
                tint = MaterialTheme.colorScheme.error
            )
        },
        title = {
            Text("Delete Item?")
        },
        text = {
            Text("Are you sure you want to delete \"$itemName\"? This action cannot be undone.")
        },
        confirmButton = {
            TextButton(
                onClick = onConfirm,
                colors = ButtonDefaults.textButtonColors(
                    contentColor = MaterialTheme.colorScheme.error
                )
            ) {
                Text("Delete")
            }
        },
        dismissButton = {
            TextButton(onClick = onDismiss) {
                Text("Cancel")
            }
        }
    )
}
```

### Modal Bottom Sheet

```kotlin
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun OptionsBottomSheet(
    onDismiss: () -> Unit,
    onOptionSelected: (String) -> Unit
) {
    val sheetState = rememberModalBottomSheetState()

    ModalBottomSheet(
        onDismissRequest = onDismiss,
        sheetState = sheetState,
        dragHandle = { BottomSheetDefaults.DragHandle() }
    ) {
        Column(
            modifier = Modifier
                .fillMaxWidth()
                .padding(bottom = 32.dp)
        ) {
            Text(
                "Options",
                modifier = Modifier.padding(horizontal = 16.dp, vertical = 8.dp),
                style = MaterialTheme.typography.titleMedium
            )

            listOf(
                Triple(Icons.Default.Share, "Share", "share"),
                Triple(Icons.Default.Edit, "Edit", "edit"),
                Triple(Icons.Default.FileCopy, "Duplicate", "duplicate"),
                Triple(Icons.Default.Delete, "Delete", "delete")
            ).forEach { (icon, label, action) ->
                ListItem(
                    headlineContent = { Text(label) },
                    leadingContent = {
                        Icon(
                            icon,
                            contentDescription = null,
                            tint = if (action == "delete")
                                MaterialTheme.colorScheme.error
                            else
                                MaterialTheme.colorScheme.onSurfaceVariant
                        )
                    },
                    modifier = Modifier.clickable { onOptionSelected(action) }
                )
            }
        }
    }
}
```

### Date and Time Pickers

```kotlin
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun DateTimePickerExample() {
    var showDatePicker by remember { mutableStateOf(false) }
    var showTimePicker by remember { mutableStateOf(false) }
    val datePickerState = rememberDatePickerState()
    val timePickerState = rememberTimePickerState()

    Column(modifier = Modifier.padding(16.dp)) {
        OutlinedButton(onClick = { showDatePicker = true }) {
            Icon(Icons.Default.CalendarToday, null)
            Spacer(Modifier.width(8.dp))
            Text(
                datePickerState.selectedDateMillis?.let {
                    SimpleDateFormat("MMM dd, yyyy", Locale.getDefault())
                        .format(Date(it))
                } ?: "Select Date"
            )
        }

        Spacer(Modifier.height(16.dp))

        OutlinedButton(onClick = { showTimePicker = true }) {
            Icon(Icons.Default.Schedule, null)
            Spacer(Modifier.width(8.dp))
            Text(
                String.format("%02d:%02d", timePickerState.hour, timePickerState.minute)
            )
        }
    }

    if (showDatePicker) {
        DatePickerDialog(
            onDismissRequest = { showDatePicker = false },
            confirmButton = {
                TextButton(onClick = { showDatePicker = false }) {
                    Text("OK")
                }
            },
            dismissButton = {
                TextButton(onClick = { showDatePicker = false }) {
                    Text("Cancel")
                }
            }
        ) {
            DatePicker(state = datePickerState)
        }
    }

    if (showTimePicker) {
        AlertDialog(
            onDismissRequest = { showTimePicker = false },
            confirmButton = {
                TextButton(onClick = { showTimePicker = false }) {
                    Text("OK")
                }
            },
            dismissButton = {
                TextButton(onClick = { showTimePicker = false }) {
                    Text("Cancel")
                }
            },
            text = {
                TimePicker(state = timePickerState)
            }
        )
    }
}
```

## Loading States

### Progress Indicators

```kotlin
@Composable
fun LoadingStates() {
    Column(
        modifier = Modifier.padding(16.dp),
        verticalArrangement = Arrangement.spacedBy(24.dp)
    ) {
        // Indeterminate circular
        CircularProgressIndicator()

        // Determinate circular
        CircularProgressIndicator(
            progress = { 0.7f },
            strokeWidth = 4.dp
        )

        // Indeterminate linear
        LinearProgressIndicator(modifier = Modifier.fillMaxWidth())

        // Determinate linear
        LinearProgressIndicator(
            progress = { 0.7f },
            modifier = Modifier.fillMaxWidth()
        )
    }
}
```

### Skeleton Loading

```kotlin
@Composable
fun SkeletonLoader(
    modifier: Modifier = Modifier
) {
    val infiniteTransition = rememberInfiniteTransition(label = "skeleton")
    val alpha by infiniteTransition.animateFloat(
        initialValue = 0.3f,
        targetValue = 0.7f,
        animationSpec = infiniteRepeatable(
            animation = tween(1000, easing = LinearEasing),
            repeatMode = RepeatMode.Reverse
        ),
        label = "alpha"
    )

    Column(
        modifier = modifier.padding(16.dp),
        verticalArrangement = Arrangement.spacedBy(12.dp)
    ) {
        repeat(5) {
            Row(
                verticalAlignment = Alignment.CenterVertically,
                horizontalArrangement = Arrangement.spacedBy(12.dp)
            ) {
                Box(
                    modifier = Modifier
                        .size(48.dp)
                        .clip(CircleShape)
                        .background(MaterialTheme.colorScheme.surfaceVariant.copy(alpha = alpha))
                )
                Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                    Box(
                        modifier = Modifier
                            .height(16.dp)
                            .fillMaxWidth(0.7f)
                            .clip(RoundedCornerShape(4.dp))
                            .background(MaterialTheme.colorScheme.surfaceVariant.copy(alpha = alpha))
                    )
                    Box(
                        modifier = Modifier
                            .height(12.dp)
                            .fillMaxWidth(0.5f)
                            .clip(RoundedCornerShape(4.dp))
                            .background(MaterialTheme.colorScheme.surfaceVariant.copy(alpha = alpha))
                    )
                }
            }
        }
    }
}
```

### Content Loading Pattern

```kotlin
@Composable
fun <T> AsyncContent(
    state: AsyncState<T>,
    onRetry: () -> Unit,
    content: @Composable (T) -> Unit
) {
    when (state) {
        is AsyncState.Loading -> {
            Box(
                modifier = Modifier.fillMaxSize(),
                contentAlignment = Alignment.Center
            ) {
                CircularProgressIndicator()
            }
        }
        is AsyncState.Success -> {
            content(state.data)
        }
        is AsyncState.Error -> {
            Column(
                modifier = Modifier
                    .fillMaxSize()
                    .padding(32.dp),
                horizontalAlignment = Alignment.CenterHorizontally,
                verticalArrangement = Arrangement.Center
            ) {
                Icon(
                    Icons.Default.Error,
                    contentDescription = null,
                    modifier = Modifier.size(64.dp),
                    tint = MaterialTheme.colorScheme.error
                )
                Spacer(Modifier.height(16.dp))
                Text(
                    "Something went wrong",
                    style = MaterialTheme.typography.titleMedium
                )
                Spacer(Modifier.height(8.dp))
                Text(
                    state.message,
                    style = MaterialTheme.typography.bodyMedium,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                    textAlign = TextAlign.Center
                )
                Spacer(Modifier.height(24.dp))
                Button(onClick = onRetry) {
                    Text("Try Again")
                }
            }
        }
    }
}

sealed class AsyncState<out T> {
    object Loading : AsyncState<Nothing>()
    data class Success<T>(val data: T) : AsyncState<T>()
    data class Error(val message: String) : AsyncState<Nothing>()
}
```

## Animations

### Animated Visibility

```kotlin
@Composable
fun ExpandableCard(
    title: String,
    content: String
) {
    var expanded by rememberSaveable { mutableStateOf(false) }

    Card(
        modifier = Modifier.fillMaxWidth()
    ) {
        Column(
            modifier = Modifier
                .clickable { expanded = !expanded }
                .padding(16.dp)
        ) {
            Row(
                modifier = Modifier.fillMaxWidth(),
                horizontalArrangement = Arrangement.SpaceBetween,
                verticalAlignment = Alignment.CenterVertically
            ) {
                Text(title, style = MaterialTheme.typography.titleMedium)
                Icon(
                    if (expanded) Icons.Default.ExpandLess else Icons.Default.ExpandMore,
                    contentDescription = if (expanded) "Collapse" else "Expand"
                )
            }

            AnimatedVisibility(
                visible = expanded,
                enter = expandVertically() + fadeIn(),
                exit = shrinkVertically() + fadeOut()
            ) {
                Text(
                    text = content,
                    modifier = Modifier.padding(top = 12.dp),
                    style = MaterialTheme.typography.bodyMedium,
                    color = MaterialTheme.colorScheme.onSurfaceVariant
                )
            }
        }
    }
}
```

### Animated Content

```kotlin
@Composable
fun AnimatedCounter(count: Int) {
    AnimatedContent(
        targetState = count,
        transitionSpec = {
            if (targetState > initialState) {
                slideInVertically { -it } + fadeIn() togetherWith
                    slideOutVertically { it } + fadeOut()
            } else {
                slideInVertically { it } + fadeIn() togetherWith
                    slideOutVertically { -it } + fadeOut()
            }.using(SizeTransform(clip = false))
        },
        label = "counter"
    ) { targetCount ->
        Text(
            text = "$targetCount",
            style = MaterialTheme.typography.displayMedium
        )
    }
}
```

### Gesture-Based Animation

```kotlin
@Composable
fun SwipeableCard(
    onSwipeLeft: () -> Unit,
    onSwipeRight: () -> Unit,
    content: @Composable () -> Unit
) {
    var offsetX by remember { mutableFloatStateOf(0f) }
    val animatedOffset by animateFloatAsState(
        targetValue = offsetX,
        animationSpec = spring(dampingRatio = Spring.DampingRatioMediumBouncy),
        label = "offset"
    )

    Card(
        modifier = Modifier
            .fillMaxWidth()
            .offset { IntOffset(animatedOffset.roundToInt(), 0) }
            .pointerInput(Unit) {
                detectHorizontalDragGestures(
                    onDragEnd = {
                        when {
                            offsetX > 200f -> {
                                onSwipeRight()
                                offsetX = 0f
                            }
                            offsetX < -200f -> {
                                onSwipeLeft()
                                offsetX = 0f
                            }
                            else -> offsetX = 0f
                        }
                    },
                    onHorizontalDrag = { _, dragAmount ->
                        offsetX += dragAmount
                    }
                )
            }
    ) {
        content()
    }
}
```


---

## อ้างอิง: details

# mobile-android-design — detailed sections

## Core Concepts

### 1. Material Design 3 Principles

**Personalization**: Dynamic color adapts UI to user's wallpaper
**Accessibility**: Tonal palettes ensure sufficient color contrast
**Large Screens**: Responsive layouts for tablets and foldables

**Material Components:**

- Cards, Buttons, FABs, Chips
- Navigation (rail, drawer, bottom nav)
- Text fields, Dialogs, Sheets
- Lists, Menus, Progress indicators

### 2. Jetpack Compose Layout System

**Column and Row:**

```kotlin
// Vertical arrangement with alignment
Column(
    modifier = Modifier.padding(16.dp),
    verticalArrangement = Arrangement.spacedBy(12.dp),
    horizontalAlignment = Alignment.Start
) {
    Text(
        text = "Title",
        style = MaterialTheme.typography.headlineSmall
    )
    Text(
        text = "Subtitle",
        style = MaterialTheme.typography.bodyMedium,
        color = MaterialTheme.colorScheme.onSurfaceVariant
    )
}

// Horizontal arrangement with weight
Row(
    modifier = Modifier.fillMaxWidth(),
    horizontalArrangement = Arrangement.SpaceBetween,
    verticalAlignment = Alignment.CenterVertically
) {
    Icon(Icons.Default.Star, contentDescription = null)
    Text("Featured")
    Spacer(modifier = Modifier.weight(1f))
    TextButton(onClick = {}) {
        Text("View All")
    }
}
```

**Lazy Lists and Grids:**

```kotlin
// Lazy column with sticky headers
LazyColumn {
    items.groupBy { it.category }.forEach { (category, categoryItems) ->
        stickyHeader {
            Text(
                text = category,
                modifier = Modifier
                    .fillMaxWidth()
                    .background(MaterialTheme.colorScheme.surface)
                    .padding(16.dp),
                style = MaterialTheme.typography.titleMedium
            )
        }
        items(categoryItems) { item ->
            ItemRow(item = item)
        }
    }
}

// Adaptive grid
LazyVerticalGrid(
    columns = GridCells.Adaptive(minSize = 150.dp),
    contentPadding = PaddingValues(16.dp),
    horizontalArrangement = Arrangement.spacedBy(12.dp),
    verticalArrangement = Arrangement.spacedBy(12.dp)
) {
    items(items) { item ->
        ItemCard(item = item)
    }
}
```

### 3. Navigation Patterns

**Bottom Navigation:**

```kotlin
@Composable
fun MainScreen() {
    val navController = rememberNavController()

    Scaffold(
        bottomBar = {
            NavigationBar {
                val navBackStackEntry by navController.currentBackStackEntryAsState()
                val currentDestination = navBackStackEntry?.destination

                NavigationDestination.entries.forEach { destination ->
                    NavigationBarItem(
                        icon = { Icon(destination.icon, contentDescription = null) },
                        label = { Text(destination.label) },
                        selected = currentDestination?.hierarchy?.any {
                            it.route == destination.route
                        } == true,
                        onClick = {
                            navController.navigate(destination.route) {
                                popUpTo(navController.graph.findStartDestination().id) {
                                    saveState = true
                                }
                                launchSingleTop = true
                                restoreState = true
                            }
                        }
                    )
                }
            }
        }
    ) { innerPadding ->
        NavHost(
            navController = navController,
            startDestination = NavigationDestination.Home.route,
            modifier = Modifier.padding(innerPadding)
        ) {
            composable(NavigationDestination.Home.route) { HomeScreen() }
            composable(NavigationDestination.Search.route) { SearchScreen() }
            composable(NavigationDestination.Profile.route) { ProfileScreen() }
        }
    }
}
```

**Navigation Drawer:**

```kotlin
@Composable
fun DrawerNavigation() {
    val drawerState = rememberDrawerState(DrawerValue.Closed)
    val scope = rememberCoroutineScope()

    ModalNavigationDrawer(
        drawerState = drawerState,
        drawerContent = {
            ModalDrawerSheet {
                Spacer(Modifier.height(12.dp))
                Text(
                    "App Name",
                    modifier = Modifier.padding(16.dp),
                    style = MaterialTheme.typography.titleLarge
                )
                HorizontalDivider()

                NavigationDrawerItem(
                    icon = { Icon(Icons.Default.Home, null) },
                    label = { Text("Home") },
                    selected = true,
                    onClick = { scope.launch { drawerState.close() } }
                )
                NavigationDrawerItem(
                    icon = { Icon(Icons.Default.Settings, null) },
                    label = { Text("Settings") },
                    selected = false,
                    onClick = { }
                )
            }
        }
    ) {
        Scaffold(
            topBar = {
                TopAppBar(
                    title = { Text("Home") },
                    navigationIcon = {
                        IconButton(onClick = { scope.launch { drawerState.open() } }) {
                            Icon(Icons.Default.Menu, contentDescription = "Menu")
                        }
                    }
                )
            }
        ) { innerPadding ->
            Content(modifier = Modifier.padding(innerPadding))
        }
    }
}
```

### 4. Material 3 Theming

**Color Scheme:**

```kotlin
// Dynamic color (Android 12+)
val dynamicColorScheme = if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.S) {
    val context = LocalContext.current
    if (darkTheme) dynamicDarkColorScheme(context)
    else dynamicLightColorScheme(context)
} else {
    if (darkTheme) DarkColorScheme else LightColorScheme
}

// Custom color scheme
private val LightColorScheme = lightColorScheme(
    primary = Color(0xFF6750A4),
    onPrimary = Color.White,
    primaryContainer = Color(0xFFEADDFF),
    onPrimaryContainer = Color(0xFF21005D),
    secondary = Color(0xFF625B71),
    onSecondary = Color.White,
    tertiary = Color(0xFF7D5260),
    onTertiary = Color.White,
    surface = Color(0xFFFFFBFE),
    onSurface = Color(0xFF1C1B1F),
)
```

**Typography:**

```kotlin
val AppTypography = Typography(
    displayLarge = TextStyle(
        fontFamily = FontFamily.Default,
        fontWeight = FontWeight.Normal,
        fontSize = 57.sp,
        lineHeight = 64.sp
    ),
    headlineMedium = TextStyle(
        fontFamily = FontFamily.Default,
        fontWeight = FontWeight.Normal,
        fontSize = 28.sp,
        lineHeight = 36.sp
    ),
    titleLarge = TextStyle(
        fontFamily = FontFamily.Default,
        fontWeight = FontWeight.Normal,
        fontSize = 22.sp,
        lineHeight = 28.sp
    ),
    bodyLarge = TextStyle(
        fontFamily = FontFamily.Default,
        fontWeight = FontWeight.Normal,
        fontSize = 16.sp,
        lineHeight = 24.sp
    ),
    labelMedium = TextStyle(
        fontFamily = FontFamily.Default,
        fontWeight = FontWeight.Medium,
        fontSize = 12.sp,
        lineHeight = 16.sp
    )
)
```

### 5. Component Examples

**Cards:**

```kotlin
@Composable
fun FeatureCard(
    title: String,
    description: String,
    imageUrl: String,
    onClick: () -> Unit
) {
    Card(
        onClick = onClick,
        modifier = Modifier.fillMaxWidth(),
        shape = RoundedCornerShape(16.dp),
        colors = CardDefaults.cardColors(
            containerColor = MaterialTheme.colorScheme.surfaceVariant
        )
    ) {
        Column {
            AsyncImage(
                model = imageUrl,
                contentDescription = null,
                modifier = Modifier
                    .fillMaxWidth()
                    .height(180.dp),
                contentScale = ContentScale.Crop
            )
            Column(modifier = Modifier.padding(16.dp)) {
                Text(
                    text = title,
                    style = MaterialTheme.typography.titleMedium
                )
                Spacer(modifier = Modifier.height(8.dp))
                Text(
                    text = description,
                    style = MaterialTheme.typography.bodyMedium,
                    color = MaterialTheme.colorScheme.onSurfaceVariant
                )
            }
        }
    }
}
```

**Buttons:**

```kotlin
// Filled button (primary action)
Button(onClick = { }) {
    Text("Continue")
}

// Filled tonal button (secondary action)
FilledTonalButton(onClick = { }) {
    Icon(Icons.Default.Add, null)
    Spacer(Modifier.width(8.dp))
    Text("Add Item")
}

// Outlined button
OutlinedButton(onClick = { }) {
    Text("Cancel")
}

// Text button
TextButton(onClick = { }) {
    Text("Learn More")
}

// FAB
FloatingActionButton(
    onClick = { },
    containerColor = MaterialTheme.colorScheme.primaryContainer,
    contentColor = MaterialTheme.colorScheme.onPrimaryContainer
) {
    Icon(Icons.Default.Add, contentDescription = "Add")
}
```


---

## อ้างอิง: material3-theming

# Material Design 3 Theming

## Color System

### Dynamic Color (Material You)

```kotlin
@Composable
fun AppTheme(
    darkTheme: Boolean = isSystemInDarkTheme(),
    dynamicColor: Boolean = true,
    content: @Composable () -> Unit
) {
    val colorScheme = when {
        dynamicColor && Build.VERSION.SDK_INT >= Build.VERSION_CODES.S -> {
            val context = LocalContext.current
            if (darkTheme) dynamicDarkColorScheme(context)
            else dynamicLightColorScheme(context)
        }
        darkTheme -> DarkColorScheme
        else -> LightColorScheme
    }

    MaterialTheme(
        colorScheme = colorScheme,
        typography = AppTypography,
        shapes = AppShapes,
        content = content
    )
}
```

### Custom Color Scheme

```kotlin
// Define color palette
val md_theme_light_primary = Color(0xFF6750A4)
val md_theme_light_onPrimary = Color(0xFFFFFFFF)
val md_theme_light_primaryContainer = Color(0xFFEADDFF)
val md_theme_light_onPrimaryContainer = Color(0xFF21005D)
val md_theme_light_secondary = Color(0xFF625B71)
val md_theme_light_onSecondary = Color(0xFFFFFFFF)
val md_theme_light_secondaryContainer = Color(0xFFE8DEF8)
val md_theme_light_onSecondaryContainer = Color(0xFF1D192B)
val md_theme_light_tertiary = Color(0xFF7D5260)
val md_theme_light_onTertiary = Color(0xFFFFFFFF)
val md_theme_light_tertiaryContainer = Color(0xFFFFD8E4)
val md_theme_light_onTertiaryContainer = Color(0xFF31111D)
val md_theme_light_error = Color(0xFFB3261E)
val md_theme_light_onError = Color(0xFFFFFFFF)
val md_theme_light_errorContainer = Color(0xFFF9DEDC)
val md_theme_light_onErrorContainer = Color(0xFF410E0B)
val md_theme_light_background = Color(0xFFFFFBFE)
val md_theme_light_onBackground = Color(0xFF1C1B1F)
val md_theme_light_surface = Color(0xFFFFFBFE)
val md_theme_light_onSurface = Color(0xFF1C1B1F)
val md_theme_light_surfaceVariant = Color(0xFFE7E0EC)
val md_theme_light_onSurfaceVariant = Color(0xFF49454F)
val md_theme_light_outline = Color(0xFF79747E)
val md_theme_light_outlineVariant = Color(0xFFCAC4D0)

val LightColorScheme = lightColorScheme(
    primary = md_theme_light_primary,
    onPrimary = md_theme_light_onPrimary,
    primaryContainer = md_theme_light_primaryContainer,
    onPrimaryContainer = md_theme_light_onPrimaryContainer,
    secondary = md_theme_light_secondary,
    onSecondary = md_theme_light_onSecondary,
    secondaryContainer = md_theme_light_secondaryContainer,
    onSecondaryContainer = md_theme_light_onSecondaryContainer,
    tertiary = md_theme_light_tertiary,
    onTertiary = md_theme_light_onTertiary,
    tertiaryContainer = md_theme_light_tertiaryContainer,
    onTertiaryContainer = md_theme_light_onTertiaryContainer,
    error = md_theme_light_error,
    onError = md_theme_light_onError,
    errorContainer = md_theme_light_errorContainer,
    onErrorContainer = md_theme_light_onErrorContainer,
    background = md_theme_light_background,
    onBackground = md_theme_light_onBackground,
    surface = md_theme_light_surface,
    onSurface = md_theme_light_onSurface,
    surfaceVariant = md_theme_light_surfaceVariant,
    onSurfaceVariant = md_theme_light_onSurfaceVariant,
    outline = md_theme_light_outline,
    outlineVariant = md_theme_light_outlineVariant
)

// Dark colors follow the same pattern
val DarkColorScheme = darkColorScheme(
    primary = md_theme_dark_primary,
    // ... other colors
)
```

### Color Roles Usage

```kotlin
@Composable
fun ColorRolesExample() {
    Column(
        modifier = Modifier.padding(16.dp),
        verticalArrangement = Arrangement.spacedBy(16.dp)
    ) {
        // Primary - Key actions, FABs
        Button(onClick = { }) {
            Text("Primary Action")
        }

        // Primary Container - Less prominent containers
        Surface(
            color = MaterialTheme.colorScheme.primaryContainer,
            shape = RoundedCornerShape(12.dp)
        ) {
            Text(
                "Primary Container",
                modifier = Modifier.padding(16.dp),
                color = MaterialTheme.colorScheme.onPrimaryContainer
            )
        }

        // Secondary - Less prominent actions
        FilledTonalButton(onClick = { }) {
            Text("Secondary Action")
        }

        // Tertiary - Contrast accents
        Badge(
            containerColor = MaterialTheme.colorScheme.tertiaryContainer,
            contentColor = MaterialTheme.colorScheme.onTertiaryContainer
        ) {
            Text("New")
        }

        // Error - Destructive actions
        Button(
            onClick = { },
            colors = ButtonDefaults.buttonColors(
                containerColor = MaterialTheme.colorScheme.error
            )
        ) {
            Text("Delete")
        }

        // Surface variants
        Surface(
            color = MaterialTheme.colorScheme.surfaceVariant,
            shape = RoundedCornerShape(8.dp)
        ) {
            Text(
                "Surface Variant",
                modifier = Modifier.padding(16.dp),
                color = MaterialTheme.colorScheme.onSurfaceVariant
            )
        }
    }
}
```

### Extended Colors

```kotlin
// Custom semantic colors beyond M3 defaults
data class ExtendedColors(
    val success: Color,
    val onSuccess: Color,
    val successContainer: Color,
    val onSuccessContainer: Color,
    val warning: Color,
    val onWarning: Color,
    val warningContainer: Color,
    val onWarningContainer: Color
)

val LocalExtendedColors = staticCompositionLocalOf {
    ExtendedColors(
        success = Color(0xFF4CAF50),
        onSuccess = Color.White,
        successContainer = Color(0xFFE8F5E9),
        onSuccessContainer = Color(0xFF1B5E20),
        warning = Color(0xFFFF9800),
        onWarning = Color.White,
        warningContainer = Color(0xFFFFF3E0),
        onWarningContainer = Color(0xFFE65100)
    )
}

@Composable
fun AppTheme(
    content: @Composable () -> Unit
) {
    val extendedColors = ExtendedColors(
        // ... define colors based on light/dark theme
    )

    CompositionLocalProvider(
        LocalExtendedColors provides extendedColors
    ) {
        MaterialTheme(
            colorScheme = colorScheme,
            content = content
        )
    }
}

// Usage
@Composable
fun SuccessBanner() {
    val extendedColors = LocalExtendedColors.current

    Surface(
        color = extendedColors.successContainer,
        shape = RoundedCornerShape(8.dp)
    ) {
        Row(
            modifier = Modifier.padding(16.dp),
            horizontalArrangement = Arrangement.spacedBy(12.dp)
        ) {
            Icon(
                Icons.Default.CheckCircle,
                contentDescription = null,
                tint = extendedColors.success
            )
            Text(
                "Operation successful!",
                color = extendedColors.onSuccessContainer
            )
        }
    }
}
```

## Typography

### Material 3 Type Scale

```kotlin
val AppTypography = Typography(
    // Display styles - Hero text, large numerals
    displayLarge = TextStyle(
        fontFamily = FontFamily.Default,
        fontWeight = FontWeight.Normal,
        fontSize = 57.sp,
        lineHeight = 64.sp,
        letterSpacing = (-0.25).sp
    ),
    displayMedium = TextStyle(
        fontFamily = FontFamily.Default,
        fontWeight = FontWeight.Normal,
        fontSize = 45.sp,
        lineHeight = 52.sp,
        letterSpacing = 0.sp
    ),
    displaySmall = TextStyle(
        fontFamily = FontFamily.Default,
        fontWeight = FontWeight.Normal,
        fontSize = 36.sp,
        lineHeight = 44.sp,
        letterSpacing = 0.sp
    ),

    // Headline styles - High emphasis, short text
    headlineLarge = TextStyle(
        fontFamily = FontFamily.Default,
        fontWeight = FontWeight.Normal,
        fontSize = 32.sp,
        lineHeight = 40.sp,
        letterSpacing = 0.sp
    ),
    headlineMedium = TextStyle(
        fontFamily = FontFamily.Default,
        fontWeight = FontWeight.Normal,
        fontSize = 28.sp,
        lineHeight = 36.sp,
        letterSpacing = 0.sp
    ),
    headlineSmall = TextStyle(
        fontFamily = FontFamily.Default,
        fontWeight = FontWeight.Normal,
        fontSize = 24.sp,
        lineHeight = 32.sp,
        letterSpacing = 0.sp
    ),

    // Title styles - Medium emphasis headers
    titleLarge = TextStyle(
        fontFamily = FontFamily.Default,
        fontWeight = FontWeight.Normal,
        fontSize = 22.sp,
        lineHeight = 28.sp,
        letterSpacing = 0.sp
    ),
    titleMedium = TextStyle(
        fontFamily = FontFamily.Default,
        fontWeight = FontWeight.Medium,
        fontSize = 16.sp,
        lineHeight = 24.sp,
        letterSpacing = 0.15.sp
    ),
    titleSmall = TextStyle(
        fontFamily = FontFamily.Default,
        fontWeight = FontWeight.Medium,
        fontSize = 14.sp,
        lineHeight = 20.sp,
        letterSpacing = 0.1.sp
    ),

    // Body styles - Long-form text
    bodyLarge = TextStyle(
        fontFamily = FontFamily.Default,
        fontWeight = FontWeight.Normal,
        fontSize = 16.sp,
        lineHeight = 24.sp,
        letterSpacing = 0.5.sp
    ),
    bodyMedium = TextStyle(
        fontFamily = FontFamily.Default,
        fontWeight = FontWeight.Normal,
        fontSize = 14.sp,
        lineHeight = 20.sp,
        letterSpacing = 0.25.sp
    ),
    bodySmall = TextStyle(
        fontFamily = FontFamily.Default,
        fontWeight = FontWeight.Normal,
        fontSize = 12.sp,
        lineHeight = 16.sp,
        letterSpacing = 0.4.sp
    ),

    // Label styles - Buttons, chips, navigation
    labelLarge = TextStyle(
        fontFamily = FontFamily.Default,
        fontWeight = FontWeight.Medium,
        fontSize = 14.sp,
        lineHeight = 20.sp,
        letterSpacing = 0.1.sp
    ),
    labelMedium = TextStyle(
        fontFamily = FontFamily.Default,
        fontWeight = FontWeight.Medium,
        fontSize = 12.sp,
        lineHeight = 16.sp,
        letterSpacing = 0.5.sp
    ),
    labelSmall = TextStyle(
        fontFamily = FontFamily.Default,
        fontWeight = FontWeight.Medium,
        fontSize = 11.sp,
        lineHeight = 16.sp,
        letterSpacing = 0.5.sp
    )
)
```

### Custom Fonts

```kotlin
// Load custom fonts
val Inter = FontFamily(
    Font(R.font.inter_regular, FontWeight.Normal),
    Font(R.font.inter_medium, FontWeight.Medium),
    Font(R.font.inter_semibold, FontWeight.SemiBold),
    Font(R.font.inter_bold, FontWeight.Bold)
)

val AppTypography = Typography(
    displayLarge = TextStyle(
        fontFamily = Inter,
        fontWeight = FontWeight.Normal,
        fontSize = 57.sp,
        lineHeight = 64.sp
    ),
    // Apply to all styles...
)

// Variable fonts (Android 12+)
val InterVariable = FontFamily(
    Font(
        R.font.inter_variable,
        variationSettings = FontVariation.Settings(
            FontVariation.weight(400)
        )
    )
)
```

## Shape System

### Material 3 Shapes

```kotlin
val AppShapes = Shapes(
    // Extra small - Chips, small buttons
    extraSmall = RoundedCornerShape(4.dp),

    // Small - Text fields, small cards
    small = RoundedCornerShape(8.dp),

    // Medium - Cards, dialogs
    medium = RoundedCornerShape(12.dp),

    // Large - Large cards, bottom sheets
    large = RoundedCornerShape(16.dp),

    // Extra large - Full-screen dialogs
    extraLarge = RoundedCornerShape(28.dp)
)
```

### Custom Shape Usage

```kotlin
@Composable
fun ShapedComponents() {
    Column(
        verticalArrangement = Arrangement.spacedBy(16.dp)
    ) {
        // Small shape for text field
        OutlinedTextField(
            value = "",
            onValueChange = {},
            shape = MaterialTheme.shapes.small,
            label = { Text("Input") }
        )

        // Medium shape for cards
        Card(
            shape = MaterialTheme.shapes.medium
        ) {
            Text("Card content", modifier = Modifier.padding(16.dp))
        }

        // Large shape for prominent containers
        Surface(
            shape = MaterialTheme.shapes.large,
            color = MaterialTheme.colorScheme.primaryContainer
        ) {
            Text("Featured", modifier = Modifier.padding(24.dp))
        }

        // Custom asymmetric shape
        Surface(
            shape = RoundedCornerShape(
                topStart = 24.dp,
                topEnd = 24.dp,
                bottomStart = 0.dp,
                bottomEnd = 0.dp
            ),
            color = MaterialTheme.colorScheme.surface
        ) {
            Text("Bottom sheet style", modifier = Modifier.padding(16.dp))
        }
    }
}
```

## Elevation and Shadows

### Tonal Elevation

```kotlin
@Composable
fun ElevationExample() {
    Column(
        verticalArrangement = Arrangement.spacedBy(16.dp)
    ) {
        // Level 0 - No elevation
        Surface(
            tonalElevation = 0.dp,
            shadowElevation = 0.dp
        ) {
            Text("Level 0", modifier = Modifier.padding(16.dp))
        }

        // Level 1 - Low emphasis surfaces
        Surface(
            tonalElevation = 1.dp,
            shadowElevation = 1.dp
        ) {
            Text("Level 1", modifier = Modifier.padding(16.dp))
        }

        // Level 2 - Cards, switches
        Surface(
            tonalElevation = 3.dp,
            shadowElevation = 2.dp
        ) {
            Text("Level 2", modifier = Modifier.padding(16.dp))
        }

        // Level 3 - Navigation components
        Surface(
            tonalElevation = 6.dp,
            shadowElevation = 4.dp
        ) {
            Text("Level 3", modifier = Modifier.padding(16.dp))
        }

        // Level 4 - Navigation rail
        Surface(
            tonalElevation = 8.dp,
            shadowElevation = 6.dp
        ) {
            Text("Level 4", modifier = Modifier.padding(16.dp))
        }

        // Level 5 - FAB
        Surface(
            tonalElevation = 12.dp,
            shadowElevation = 8.dp
        ) {
            Text("Level 5", modifier = Modifier.padding(16.dp))
        }
    }
}
```

## Responsive Design

### Window Size Classes

```kotlin
@Composable
fun AdaptiveLayout() {
    val windowSizeClass = calculateWindowSizeClass(LocalContext.current as Activity)

    when (windowSizeClass.widthSizeClass) {
        WindowWidthSizeClass.Compact -> {
            // Phone portrait - Single column, bottom nav
            CompactLayout()
        }
        WindowWidthSizeClass.Medium -> {
            // Tablet portrait, phone landscape - Navigation rail
            MediumLayout()
        }
        WindowWidthSizeClass.Expanded -> {
            // Tablet landscape, desktop - Navigation drawer, multi-pane
            ExpandedLayout()
        }
    }
}

@Composable
fun CompactLayout() {
    Scaffold(
        bottomBar = { NavigationBar { /* items */ } }
    ) { padding ->
        Content(modifier = Modifier.padding(padding))
    }
}

@Composable
fun MediumLayout() {
    Row {
        NavigationRail { /* items */ }
        Content(modifier = Modifier.weight(1f))
    }
}

@Composable
fun ExpandedLayout() {
    PermanentNavigationDrawer(
        drawerContent = {
            PermanentDrawerSheet { /* items */ }
        }
    ) {
        Row {
            ListPane(modifier = Modifier.weight(0.4f))
            DetailPane(modifier = Modifier.weight(0.6f))
        }
    }
}
```

### Foldable Support

```kotlin
@Composable
fun FoldableAwareLayout() {
    val foldingFeature = LocalFoldingFeature.current

    when {
        foldingFeature?.state == FoldingFeature.State.HALF_OPENED -> {
            // Device is half-folded (tabletop mode)
            TwoHingeLayout(
                top = { CameraPreview() },
                bottom = { CameraControls() }
            )
        }
        foldingFeature?.orientation == FoldingFeature.Orientation.VERTICAL -> {
            // Vertical fold (book mode)
            TwoPaneLayout(
                first = { NavigationPane() },
                second = { ContentPane() }
            )
        }
        else -> {
            // Regular or fully opened
            SinglePaneLayout()
        }
    }
}
```
