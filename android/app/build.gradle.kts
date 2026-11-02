plugins {
    id("com.android.application")
    id("org.jetbrains.kotlin.android")
}

android {
    namespace = "com.cliprecorder.sync"
    compileSdk = 34

    defaultConfig {
        applicationId = "com.cliprecorder.sync"
        minSdk = 26
        targetSdk = 34
        versionCode = 1
        versionName = "1.0"
    }

    buildTypes {
        getByName("release") {
            isMinifyEnabled = false
        }
    }

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }

    kotlinOptions {
        jvmTarget = "17"
    }
}

dependencies {
    implementation("com.journeyapps:zxing-android-embedded:4.3.0")
    // The scanner references androidx.core (ContextCompat/ActivityCompat),
    // androidx.activity and androidx.fragment but its POM only declares the
    // zxing core; without these the scan screen dies with NoClassDefFoundError.
    implementation("androidx.appcompat:appcompat:1.7.0")
}
