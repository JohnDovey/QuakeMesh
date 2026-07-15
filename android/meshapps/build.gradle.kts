plugins {
    alias(libs.plugins.android.application)
    alias(libs.plugins.kotlin.android)
}

android {
    namespace = "net.quakemesh.meshapps.standalone"
    compileSdk = 35

    defaultConfig {
        applicationId = "net.quakemesh.meshapps"
        minSdk = 26
        targetSdk = 35
        versionCode = 102
        versionName = "1.0.2"
    }

    buildFeatures {
        buildConfig = true
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
    implementation(libs.androidx.core.ktx)
    implementation(libs.androidx.appcompat)
    implementation(project(":meshapps-lib"))
}
